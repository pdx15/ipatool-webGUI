package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/byteness/keyring"
	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/majd/ipatool/v2/pkg/keychain"
	"github.com/majd/ipatool/v2/pkg/util"
	"github.com/majd/ipatool/v2/pkg/util/machine"
	"github.com/majd/ipatool/v2/pkg/util/operatingsystem"
	"github.com/majd/ipatool/v2/resources"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

//go:embed gui_assets/*
var embeddedAssets embed.FS

// DownloadJob represents an asynchronous download task tracked by the GUI.
type DownloadJob struct {
	ID         string  `json:"id"`
	AppName    string  `json:"appName"`
	BundleID   string  `json:"bundleId"`
	AppID      int64   `json:"appId"`
	Platform   string  `json:"platform,omitempty"`
	Version    string  `json:"version"`
	Progress   float64 `json:"progress"`
	BytesRead  int64   `json:"bytesRead"`
	TotalBytes int64   `json:"totalBytes"`
	Speed      string  `json:"speed"`
	Status     string  `json:"status"` // "queued", "purchasing", "downloading", "patching", "completed", "error"
	Error      string  `json:"error,omitempty"`
	Warning    string  `json:"warning,omitempty"` // non-fatal issue, job still completed
	OutputPath string  `json:"outputPath,omitempty"`
	CreatedAt  int64   `json:"createdAt"`
	// progress is the ipatool progress bar feeding this job. It is sampled
	// whenever the job is serialized for a status response and never exposed.
	progress *downloadProgress `json:"-"`
}

// noSinfWarning is shown when Apple served the package without DRM
// signatures: the .ipa is complete but cannot be installed on a device.
const noSinfWarning = "Apple returned the package without DRM signatures (sinf). " +
	"The .ipa was saved and can be decrypted/inspected, but it will not install on a device. " +
	"Try again later or download a different version."

type jobTracker struct {
	sync.RWMutex
	jobs map[string]*DownloadJob
}

var activeJobs = &jobTracker{
	jobs: make(map[string]*DownloadJob),
}

// versionMetadataCache caches per-build display version, release date and
// minimum OS version so revisiting the version-history table does not re-read
// each IPA from Apple.
type versionMetadataCache struct {
	sync.RWMutex
	m map[string]map[string]versionMetaEntry // appID -> versionID -> entry
}

type versionMetaEntry struct {
	DisplayVersion   string
	ReleaseDate      string
	MinimumOSVersion string
}

var versionMetaCache = &versionMetadataCache{
	m: make(map[string]map[string]versionMetaEntry),
}

func (jt *jobTracker) get(id string) (*DownloadJob, bool) {
	jt.Lock()
	defer jt.Unlock()
	j, exists := jt.jobs[id]
	if !exists {
		return nil, false
	}
	if j.progress != nil {
		j.progress.sample(j)
	}
	// return copy
	copyJob := *j
	return &copyJob, true
}

func (jt *jobTracker) set(j *DownloadJob) {
	jt.Lock()
	defer jt.Unlock()
	jt.jobs[j.ID] = j
}

// update mutates a stored job while holding the tracker lock, so concurrent
// readers never observe a job that is only half updated.
func (jt *jobTracker) update(id string, fn func(*DownloadJob)) {
	jt.Lock()
	defer jt.Unlock()

	if j, exists := jt.jobs[id]; exists {
		fn(j)
	}
}

// downloadProgress adapts ipatool's download progress bar to the GUI job
// status. ipatool feeds every downloaded byte into the bar it is given, so the
// GUI only has to sample that bar to report size, percentage and speed. The bar
// itself renders nothing: progress reaches the browser through /api/download.
type downloadProgress struct {
	bar        *progressbar.ProgressBar
	lastSample time.Time
	lastBytes  int64
	speed      string
	mu         sync.Mutex
}

// progressBarUnsetMax is the max the bar starts with: ipatool's CLI creates its
// download bar the same way and replaces the value with the real size once the
// response headers arrive. Until then the size is unknown, which the sample
// below recognises by a max that cannot be a real package size.
const progressBarUnsetMax = 1

// newDownloadProgress creates the progress bar ipatool updates during a
// download. Output is discarded because the GUI reports progress over HTTP
// instead of rendering a terminal bar, and rendering is throttled because the
// sampled state is at most a few hundred milliseconds old anyway.
func newDownloadProgress() *downloadProgress {
	return &downloadProgress{
		bar: progressbar.NewOptions64(progressBarUnsetMax,
			progressbar.OptionSetWriter(io.Discard),
			progressbar.OptionThrottle(500*time.Millisecond),
			progressbar.OptionSetElapsedTime(false),
			progressbar.OptionSetPredictTime(false),
		),
		lastSample: time.Now(),
	}
}

// sample copies the current bar state into the job. It runs whenever a status
// endpoint reads a job, so the reported values are always fresh.
func (dp *downloadProgress) sample(job *DownloadJob) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	// ipatool counts every byte it writes into the bar and calls ChangeMax64
	// with the package size as soon as the response reports it.
	bytesRead := int64(dp.bar.State().CurrentBytes)
	total := dp.bar.GetMax64()

	now := time.Now()
	if elapsed := now.Sub(dp.lastSample).Seconds(); elapsed >= 0.3 {
		if delta := bytesRead - dp.lastBytes; delta >= 0 {
			dp.speed = formatTransferSpeed(float64(delta) / elapsed)
		}
		dp.lastSample = now
		dp.lastBytes = bytesRead
	}

	job.BytesRead = bytesRead
	if total > progressBarUnsetMax {
		job.TotalBytes = total
		job.Progress = float64(bytesRead) / float64(total) * 100.0
	}
	if dp.speed != "" {
		job.Speed = dp.speed
	}
}

// formatTransferSpeed renders a byte-per-second rate the way the GUI shows it.
func formatTransferSpeed(bytesPerSecond float64) string {
	switch {
	case bytesPerSecond > 1024*1024:
		return fmt.Sprintf("%.2f MB/s", bytesPerSecond/(1024*1024))
	case bytesPerSecond > 1024:
		return fmt.Sprintf("%.1f KB/s", bytesPerSecond/1024)
	default:
		return fmt.Sprintf("%d B/s", int64(bytesPerSecond))
	}
}

const (
	guiConfigDirectoryName = ".ipatool-webgui"
	guiKeychainServiceName = "ipatool-webgui-auth.service"
)

func prepareGUIStateDirectory(osImpl operatingsystem.OperatingSystem, homeDirectory string) (string, error) {
	stateDirectory := filepath.Join(homeDirectory, guiConfigDirectoryName)
	if err := osImpl.MkdirAll(stateDirectory, 0700); err != nil {
		return "", fmt.Errorf("failed to create webgui state directory: %w", err)
	}

	return stateDirectory, nil
}

func newGUIKeychain(stateDirectory string, interactive bool) keychain.Keychain {
	ring := util.Must(openKeyring(keyring.Config{
		AllowedBackends: []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.SecretServiceBackend,
			keyring.FileBackend,
		},
		ServiceName:              guiKeychainServiceName,
		KeychainTrustApplication: true,
		FileDir:                  stateDirectory,
		FilePasswordFunc: func(s string) (string, error) {
			if keychainPassphrase == "" && !interactive {
				return "", errors.New("keychain passphrase is required when not running in interactive mode; use the \"--keychain-passphrase\" flag")
			}

			if keychainPassphrase != "" {
				return keychainPassphrase, nil
			}

			path := strings.Split(s, " unlock ")[1]
			password, err := readPrompt(fmt.Sprintf("enter passphrase to unlock %s (this is separate from your Apple ID password): ", path), true)
			if err != nil {
				return "", fmt.Errorf("failed to read password: %w", err)
			}

			return password, nil
		},
	}))

	return keychain.New(keychain.Args{
		Keyring: ring,
		Label:   guiKeychainServiceName,
	})
}

func initGUIWithCommand(cmd *cobra.Command) error {
	verbose := cmd.Flag("verbose") != nil && cmd.Flag("verbose").Value.String() == "true"
	nonInteractive := cmd.Flag("non-interactive") != nil && cmd.Flag("non-interactive").Value.String() == "true"
	ctx := context.WithValue(context.Background(), interactiveKey, !nonInteractive)
	cmd.SetContext(ctx)

	format := OutputFormatText
	if formatFlag := cmd.Flag("format"); formatFlag != nil && formatFlag.Value.String() != "" {
		if parsed, err := OutputFormatFromString(formatFlag.Value.String()); err == nil {
			format = parsed
		}
	}

	dependencies.Logger = newLogger(format, verbose)
	dependencies.OS = operatingsystem.New()
	dependencies.Machine = machine.New(machine.Args{OS: dependencies.OS})

	stateDirectory, err := prepareGUIStateDirectory(dependencies.OS, dependencies.Machine.HomeDirectory())
	if err != nil {
		return err
	}

	if err := ensureGUIKeychainPassphrase(); err != nil {
		return err
	}

	dependencies.CookieJar = newGUICookieJar(stateDirectory)
	dependencies.Keychain = newGUIKeychain(stateDirectory, !nonInteractive)
	dependencies.AppStore = appstore.NewAppStore(appstore.Args{
		CookieJar:       dependencies.CookieJar,
		OperatingSystem: dependencies.OS,
		Keychain:        dependencies.Keychain,
		Machine:         dependencies.Machine,
	})

	return nil
}

func guiCmd() *cobra.Command {
	var (
		host      string
		port      int
		noBrowser bool
	)

	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Launch the graphical user interface for ipatool",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initGUIWithCommand(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGUIServer(host, port, noBrowser)
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host address to bind GUI server to")
	cmd.Flags().IntVarP(&port, "port", "p", 54321, "Port to run GUI server on (0 for random available port)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Do not automatically open default web browser on start")

	return cmd
}

func runGUIServer(host string, port int, noBrowser bool) error {
	if dir := appWorkingDir(); dir != "" && dir != "." {
		_ = os.Chdir(dir)
	}

	// Setup asset filesystem
	assetsSub, err := fs.Sub(embeddedAssets, "gui_assets")
	if err != nil {
		return fmt.Errorf("failed to open embedded assets: %w", err)
	}

	mux := http.NewServeMux()

	// Static assets handler
	assetsFS := http.FS(assetsSub)
	fileServer := http.FileServer(assetsFS)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, readErr := fs.ReadFile(assetsSub, "index.html")
			if readErr != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}

		if r.URL.Path == "/assets/style.css" {
			data, readErr := fs.ReadFile(assetsSub, "style.css")
			if readErr != nil {
				http.Error(w, "style.css not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = w.Write(data)
			return
		}

		if r.URL.Path == "/assets/app.js" {
			data, readErr := fs.ReadFile(assetsSub, "app.js")
			if readErr != nil {
				http.Error(w, "app.js not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(data)
			return
		}

		fileServer.ServeHTTP(w, r)
	})

	// API Handlers
	mux.HandleFunc("/api/status", handleAPIStatus)
	mux.HandleFunc("/api/update-check", handleAPIUpdateCheck)
	mux.HandleFunc("/api/auth/login", handleAPILogin)
	mux.HandleFunc("/api/auth/login/mzfinance", handleAPILoginMZFinance)
	mux.HandleFunc("/api/auth/revoke", handleAPIRevoke)
	mux.HandleFunc("/api/search", handleAPISearch)
	mux.HandleFunc("/api/search/all", handleAPISearchAll)
	mux.HandleFunc("/api/removed-apps", handleAPIRemovedApps)
	mux.HandleFunc("/api/qrcode", handleAPIQRCode)
	mux.HandleFunc("/api/purchase", handleAPIPurchase)
	mux.HandleFunc("/api/download", handleAPIDownload)
	mux.HandleFunc("/api/download/status", handleAPIDownloadStatus)
	mux.HandleFunc("/api/icloud/status", handleAPIICloudStatus)
	mux.HandleFunc("/api/downloads/active", handleAPIActiveDownloads)
	mux.HandleFunc("/api/versions", handleAPIVersions)
	mux.HandleFunc("/api/version-metadata", handleAPIVersionMetadata)
	mux.HandleFunc("/api/version-metadata/batch", handleAPIVersionMetadataBatch)
	mux.HandleFunc("/api/purchases", handleAPIPurchases)
	mux.HandleFunc("/api/save-list", handleAPISaveList)
	mux.HandleFunc("/api/batch/check", handleAPIBatchCheck)
	mux.HandleFunc("/api/batch/check/status", handleAPIBatchCheckStatus)
	mux.HandleFunc("/api/batch/download", handleAPIBatchDownload)
	mux.HandleFunc("/api/batch/download/status", handleAPIBatchDownloadStatus)
	mux.HandleFunc("/api/batch/download/skip", handleAPIBatchDownloadSkip)
	mux.HandleFunc("/api/install/devices", handleAPIInstallDevices)
	mux.HandleFunc("/api/install/upload", handleAPIInstallUpload)
	mux.HandleFunc("/api/install/status", handleAPIInstallStatus)
	mux.HandleFunc("/api/open-folder", handleAPIOpenFolder)

	// Wrap mux with CORS headers for local/preview access
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		mux.ServeHTTP(w, r)
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return fmt.Errorf("failed to bind server to %s:%d: %w", host, port, err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	serverURL := fmt.Sprintf("http://localhost:%d", actualPort)
	if host != "127.0.0.1" && host != "localhost" {
		serverURL = fmt.Sprintf("http://%s:%d", host, actualPort)
	}

	fmt.Printf("\n=======================================================\n")
	fmt.Printf("   ipatool GUI is running!\n")
	fmt.Printf("   URL: %s\n", serverURL)
	fmt.Printf("   Press Ctrl+C to stop the GUI server.\n")
	fmt.Printf("=======================================================\n\n")

	if !noBrowser {
		go openBrowser(serverURL)
	}

	server := &http.Server{
		Handler: handler,
	}

	return server.Serve(listener)
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)

	switch runtime.GOOS {
	case "windows":
		// On Windows, try Edge App Mode first for a sleek desktop app window!
		edgePath := os.ExpandEnv(`%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe`)
		if _, err := os.Stat(edgePath); err == nil {
			_ = exec.Command(edgePath, fmt.Sprintf("--app=%s", url)).Start()
			return
		}
		edgePath64 := os.ExpandEnv(`%ProgramFiles%\Microsoft\Edge\Application\msedge.exe`)
		if _, err := os.Stat(edgePath64); err == nil {
			_ = exec.Command(edgePath64, fmt.Sprintf("--app=%s", url)).Start()
			return
		}
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}

// API JSON Helpers
func jsonResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, statusCode int, message string) {
	jsonResponse(w, statusCode, map[string]interface{}{
		"success": false,
		"message": message,
	})
}

// ==========================================
// API Handlers
// ==========================================

func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	info, err := dependencies.AppStore.AccountInfo()
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
			"account":       nil,
			"version":       version,
			"os":            runtime.GOOS,
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"account": map[string]interface{}{
			"name":       info.Account.Name,
			"email":      info.Account.Email,
			"storefront": info.Account.StoreFront,
			"dsid":       info.Account.DirectoryServicesID,
		},
		"version": version,
		"os":      runtime.GOOS,
	})
}

const (
	webGUIRepoReleasesURL       = "https://github.com/pdx15/ipatool-webGUI/releases"
	webGUIRepoLatestRedirectURL = "https://github.com/pdx15/ipatool-webGUI/releases/latest"
	webGUIRepoLatestAPIURL      = "https://api.github.com/repos/pdx15/ipatool-webGUI/releases/latest"
	updateCheckCacheTTL         = 15 * time.Minute
)

var updateCheckState = struct {
	sync.Mutex
	latestTag  string
	releaseURL string
	checkedAt  time.Time
}{}

func handleAPIUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	currentClean := normalizeWebGUIVersion(version)
	latestTag, releaseURL := fetchLatestWebGUIRelease(r.URL.Query().Get("force") == "1")
	latestClean := normalizeWebGUIVersion(latestTag)
	if releaseURL == "" {
		releaseURL = webGUIRepoReleasesURL
	}

	updateAvailable := false
	if currentClean != "" && currentClean != "dev" && latestClean != "" {
		updateAvailable = compareWebGUIVersions(latestClean, currentClean) > 0
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":         true,
		"currentVersion":  version,
		"displayVersion":  currentClean,
		"latestTag":       latestTag,
		"latestVersion":   latestClean,
		"updateAvailable": updateAvailable,
		"releasesUrl":     releaseURL,
	})
}

func fetchLatestWebGUIRelease(force bool) (string, string) {
	updateCheckState.Lock()
	if !force && updateCheckState.latestTag != "" && time.Since(updateCheckState.checkedAt) < updateCheckCacheTTL {
		tag, relURL := updateCheckState.latestTag, updateCheckState.releaseURL
		updateCheckState.Unlock()
		return tag, relURL
	}
	updateCheckState.Unlock()

	tag, relURL := queryGitHubLatestReleaseAPI()
	if tag == "" {
		tag, relURL = queryGitHubLatestReleaseRedirect()
	}

	if tag != "" {
		updateCheckState.Lock()
		updateCheckState.latestTag = tag
		updateCheckState.releaseURL = relURL
		updateCheckState.checkedAt = time.Now()
		updateCheckState.Unlock()
	}

	return tag, relURL
}

func queryGitHubLatestReleaseAPI() (string, string) {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodGet, webGUIRepoLatestAPIURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ipatool-webGUI-update-checker")

	res, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return "", ""
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", ""
	}

	return strings.TrimSpace(payload.TagName), strings.TrimSpace(payload.HTMLURL)
}

func queryGitHubLatestReleaseRedirect() (string, string) {
	client := &http.Client{
		Timeout: 6 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodGet, webGUIRepoLatestRedirectURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "ipatool-webGUI-update-checker")

	res, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	loc := strings.TrimSpace(res.Header.Get("Location"))
	if idx := strings.LastIndex(loc, "/releases/tag/"); idx >= 0 {
		tag := strings.TrimSpace(loc[idx+len("/releases/tag/"):])
		if tag != "" {
			return tag, loc
		}
	}

	return "", ""
}

func normalizeWebGUIVersion(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	lower := strings.ToLower(v)
	if idx := strings.Index(lower, "-webgui"); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}

func parseWebGUIVersionParts(v string) ([]int, string, bool) {
	clean := normalizeWebGUIVersion(v)
	if clean == "" || strings.EqualFold(clean, "dev") {
		return nil, "", false
	}

	idx := len(clean)
	for idx > 0 {
		ch := clean[idx-1]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			idx--
		} else {
			break
		}
	}

	numPart := clean[:idx]
	suffix := strings.ToLower(clean[idx:])
	rawParts := strings.Split(numPart, ".")
	if len(rawParts) == 0 {
		return nil, "", false
	}

	nums := make([]int, 0, 3)
	for _, p := range rawParts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, "", false
		}
		nums = append(nums, n)
	}
	for len(nums) < 3 {
		nums = append(nums, 0)
	}

	return nums, suffix, true
}

func compareWebGUIVersions(a, b string) int {
	aNums, aSuffix, aOK := parseWebGUIVersionParts(a)
	bNums, bSuffix, bOK := parseWebGUIVersionParts(b)
	if !aOK || !bOK {
		return 0
	}

	maxLen := len(aNums)
	if len(bNums) > maxLen {
		maxLen = len(bNums)
	}
	for i := 0; i < maxLen; i++ {
		var av, bv int
		if i < len(aNums) {
			av = aNums[i]
		}
		if i < len(bNums) {
			bv = bNums[i]
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}

	if aSuffix != bSuffix {
		if aSuffix > bSuffix {
			return 1
		}
		return -1
	}

	return 0
}

type loginRequestPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AuthCode string `json:"authCode"`
}

func handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req loginRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if req.Email == "" || req.Password == "" {
		jsonError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	output, err := dependencies.AppStore.Login(appstore.LoginInput{
		Email:    req.Email,
		Password: req.Password,
		AuthCode: req.AuthCode,
	})

	if err != nil {
		if errors.Is(err, appstore.ErrAuthCodeRequired) {
			jsonResponse(w, http.StatusOK, map[string]interface{}{
				"success":          false,
				"authCodeRequired": true,
				"message":          "2FA verification code is required",
			})
			return
		}

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"account": map[string]interface{}{
			"name":       output.Account.Name,
			"email":      output.Account.Email,
			"storefront": output.Account.StoreFront,
			"dsid":       output.Account.DirectoryServicesID,
		},
	})
}

// handleAPILoginMZFinance is an alternative login endpoint for GUI compatibility.
func handleAPILoginMZFinance(w http.ResponseWriter, r *http.Request) {
	handleAPILogin(w, r)
}

func handleAPIRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	err := dependencies.AppStore.Revoke()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	invalidatePurchasesCache()

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// searchLimit parses the "limit" query parameter, falling back to def.
func searchLimit(r *http.Request, def int64) int64 {
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}

	return def
}

// searchPlatform parses the "platform" query parameter, defaulting to iPhone
// for unknown or missing values.
func searchPlatform(r *http.Request) appstore.Platform {
	platform, err := appstore.ParsePlatform(r.URL.Query().Get("platform"))
	if err != nil {
		return appstore.PlatformIPhone
	}

	return platform
}

// appStoreSearch queries the official App Store on behalf of the GUI.
func appStoreSearch(term string, platform appstore.Platform, limit int64) (appstore.SearchOutput, error) {
	infoResult, err := dependencies.AppStore.AccountInfo()
	var acc appstore.Account
	if err == nil {
		acc = infoResult.Account
	} else {
		// Fallback to default US storefront if not logged in
		acc = appstore.Account{StoreFront: "143441-1,29"}
	}

	return dependencies.AppStore.Search(appstore.SearchInput{
		Account:  acc,
		Term:     term,
		Limit:    limit,
		Platform: platform,
	})
}

func handleAPISearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	term := r.URL.Query().Get("term")
	if term == "" {
		jsonError(w, http.StatusBadRequest, "search term is required")
		return
	}

	platform := searchPlatform(r)
	out, err := appStoreSearch(term, platform, searchLimit(r, 25))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   out.Count,
		"results": appCardsForPlatform(out.Results, platform),
	})
}

// handleAPISearchAll answers both result groups of the app search in one
// request: the apps that were removed from the App Store (Apps_ID_List.txt
// catalog) and the official App Store results. The removed apps are listed
// first in the response and in the GUI, because they are the harder half to
// find — the official search never returns them.
//
// It exists so the ordering of the two groups is decided server side instead of
// relying on the frontend firing two parallel requests, whose responses may
// arrive in any order. /api/search and /api/removed-apps remain available.
func handleAPISearchAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	term := r.URL.Query().Get("term")
	if strings.TrimSpace(term) == "" {
		jsonError(w, http.StatusBadRequest, "search term is required")
		return
	}

	platform := searchPlatform(r)
	limit := searchLimit(r, 25)

	removedResults := searchRemovedApps(term, limit)
	out, searchErr := appStoreSearch(term, platform, limit)
	if searchErr != nil && len(removedResults) == 0 {
		jsonError(w, http.StatusInternalServerError, searchErr.Error())
		return
	}

	official := map[string]interface{}{
		"count":   out.Count,
		"results": appCardsForPlatform(out.Results, platform),
	}

	if searchErr != nil {
		// Still show the removed apps found locally, but tell the UI why the
		// official half of the results is missing.
		official["count"] = 0
		official["results"] = []appCard{}
		official["error"] = searchErr.Error()
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"removed": map[string]interface{}{
			"count":   len(removedResults),
			"results": removedResults,
		},
		"official": official,
	})
}

// removedAppEntry is one app from the Apps_ID_List.txt catalog. These apps are
// no longer discoverable through the App Store search, but can still be
// downloaded directly when their numeric App ID is known.
type removedAppEntry struct {
	AppID int64  `json:"appId"`
	Name  string `json:"name"`
}

// handleAPIRemovedApps searches the Apps_ID_List.txt catalog (apps removed
// from the App Store but still downloadable by ID) by name or by numeric App
// ID. It is kept separate from /api/search for clients that only need this
// group; /api/search/all answers with both groups in the order the GUI shows
// them (removed apps first).
func handleAPIRemovedApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	results := searchRemovedApps(r.URL.Query().Get("term"), searchLimit(r, 50))

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   len(results),
		"results": results,
	})
}

// searchRemovedApps matches term against the Apps_ID_List.txt catalog by name
// (case-insensitive substring) or by exact numeric App ID. The most likely
// candidate is returned first: exact App ID, then exact name, then names that
// start with the term, then plain substring hits.
func searchRemovedApps(term string, limit int64) []removedAppEntry {
	term = strings.ToLower(strings.TrimSpace(term))

	var termID int64
	termIsNumeric := false
	if term != "" {
		if v, err := strconv.ParseInt(term, 10, 64); err == nil && v > 0 {
			termID = v
			termIsNumeric = true
		}
	}

	type rankedEntry struct {
		entry removedAppEntry
		rank  int
	}

	ranked := []rankedEntry{}

	for _, entry := range loadRemovedApps() {
		rank := removedMatchRank(entry, term, termID, termIsNumeric)
		if rank < 0 {
			continue
		}

		ranked = append(ranked, rankedEntry{entry: entry, rank: rank})
	}

	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].rank < ranked[j].rank })

	if limit > 0 && int64(len(ranked)) > limit {
		ranked = ranked[:limit]
	}

	results := make([]removedAppEntry, 0, len(ranked))

	for _, match := range ranked {
		results = append(results, match.entry)
	}

	return results
}

// removedMatchRank scores one catalog entry against the search term. Lower is
// better; a negative rank means the entry does not match at all.
func removedMatchRank(entry removedAppEntry, term string, termID int64, termIsNumeric bool) int {
	switch {
	case termIsNumeric && entry.AppID == termID:
		return removedRankExactID
	case term == "":
		// No term: every app matches, so the catalog order is kept.
		return removedRankSubstring
	}

	name := strings.ToLower(strings.TrimSpace(entry.Name))
	switch {
	case name == term:
		return removedRankExactName
	case strings.HasPrefix(name, term):
		return removedRankNamePrefix
	case strings.Contains(name, term):
		return removedRankSubstring
	default:
		return -1
	}
}

// Match quality of a catalog entry, from best to worst.
const (
	removedRankExactID    = 0
	removedRankExactName  = 1
	removedRankNamePrefix = 2
	removedRankSubstring  = 3
)

// handleAPIQRCode serves the donate QR code embedded in the binary.
func handleAPIQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	data, err := resources.FS.ReadFile("qrCode.png")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// appsIDListDirs returns the directories where Apps_ID_List.txt may live: the
// process working directory followed by the directory of the running binary.
func appsIDListDirs() []string {
	dirs := make([]string, 0, 2)
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	return dirs
}

// loadRemovedApps resolves the removed-apps catalog. An Apps_ID_List.txt on
// disk — in the working directory or next to the running binary — wins, so the
// catalog can be replaced without rebuilding. Otherwise the copy compiled into
// the binary is used, which is what makes single-file builds (the macOS
// releases) ship the removed-apps search. A missing or unparsable catalog is
// not an error: the rest of the GUI keeps working and the search simply shows
// no removed apps.
func loadRemovedApps() []removedAppEntry {
	for _, dir := range appsIDListDirs() {
		data, err := os.ReadFile(filepath.Join(dir, "Apps_ID_List.txt"))
		if err != nil {
			continue
		}
		if entries := parseAppsIDList(string(data)); len(entries) > 0 {
			return entries
		}
	}

	if entries := parseAppsIDList(resources.AppsIDList); len(entries) > 0 {
		return entries
	}

	return []removedAppEntry{}
}

// parseAppsIDList parses the Apps_ID_List.txt catalog. Each line is expected
// to look like "1Password 7: 568903335", but plain IDs, App Store URLs and
// ":", "-", ";", tab or space separators are also tolerated.
func parseAppsIDList(content string) []removedAppEntry {
	entries := []removedAppEntry{}
	seen := map[int64]bool{}

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		idStr, name := extractAppID(line)
		if idStr == "" {
			continue
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true

		if name == "" {
			name = idStr
		}
		entries = append(entries, removedAppEntry{AppID: id, Name: name})
	}

	return entries
}

// extractAppID pulls a numeric App ID out of a list line, returning the ID
// string and the remaining name part. App Store URLs (https://.../id123456789)
// are detected first; otherwise the trailing number is used as the ID.
func extractAppID(line string) (id, name string) {
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "http") || strings.Contains(lower, "apps.apple.com") {
		if i := strings.Index(lower, "/id"); i >= 0 {
			rest := line[i+3:]
			j := 0
			for j < len(rest) && isASCIIDigit(rest[j]) {
				j++
			}
			if j >= 4 {
				return rest[:j], ""
			}
		}
	}

	// Locate the trailing run of digits and take it as the App ID.
	cut := len(line)
	for cut > 0 && isASCIIDigit(line[cut-1]) {
		cut--
	}
	if cut == len(line) {
		return "", ""
	}

	id = line[cut:]
	if len(id) < 4 {
		return "", ""
	}

	name = strings.TrimSpace(line[:cut])
	name = strings.Trim(name, ":;,-–—\t ")
	return id, name
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

type purchaseRequestPayload struct {
	BundleID string `json:"bundleId"`
	Platform string `json:"platform"`
}

func handleAPIPurchase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req purchaseRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	platform, _ := appstore.ParsePlatform(req.Platform)

	info, err := dependencies.AppStore.AccountInfo()
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "you must log in to purchase apps")
		return
	}

	lookup, err := dependencies.AppStore.Lookup(appstore.LookupInput{
		Account:  info.Account,
		BundleID: req.BundleID,
		Platform: platform,
	})
	if err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("app lookup failed: %v", err))
		return
	}

	_, alreadyOwned, purchaseErr := purchaseWithRetry(info.Account, lookup.App, platform)
	if purchaseErr != nil {
		jsonError(w, http.StatusInternalServerError, purchaseErr.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"alreadyOwned": alreadyOwned,
	})
}

type downloadRequestPayload struct {
	BundleID          string `json:"bundleId"`
	AppID             int64  `json:"appId"`
	AppName           string `json:"appName"`
	OutputPath        string `json:"outputPath"`
	ExternalVersionID string `json:"externalVersionId"`
	Platform          string `json:"platform"`
	Purchase          bool   `json:"purchase"`
}

const defaultDownloadFolderName = "apps"

func isTestOrSystemExecDir(exe string, dir string) bool {
	base := filepath.Base(exe)
	if strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe") || strings.Contains(exe, "go-build") {
		return true
	}
	clean := filepath.Clean(dir)
	for _, prefix := range []string{"/usr/", "/bin", "/sbin", "/opt/", "/tmp", "/private/tmp", "/var/folders/"} {
		if clean == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

func appWorkingDir() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		if resolved, evalErr := filepath.EvalSymlinks(exe); evalErr == nil && resolved != "" {
			exe = resolved
		}
		exeDir := filepath.Clean(filepath.Dir(exe))
		if exeDir != "" && exeDir != "." && !isTestOrSystemExecDir(exe, exeDir) {
			return exeDir
		}
	}

	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		return filepath.Clean(cwd)
	}

	return "."
}

func defaultDownloadDir() string {
	dir := filepath.Join(appWorkingDir(), defaultDownloadFolderName)
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func resolveDownloadOutputPath(requested string) string {
	trimmed := strings.TrimSpace(requested)
	if trimmed == "" {
		return defaultDownloadDir()
	}
	return trimmed
}

func handleAPIDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req downloadRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if req.AppID == 0 && req.BundleID == "" {
		jsonError(w, http.StatusBadRequest, "either app ID or bundle ID is required")
		return
	}

	info, err := dependencies.AppStore.AccountInfo()
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "you must log in to download apps")
		return
	}

	outputPath := resolveDownloadOutputPath(req.OutputPath)

	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())
	job := &DownloadJob{
		ID:         jobID,
		BundleID:   req.BundleID,
		AppID:      req.AppID,
		AppName:    req.AppName,
		Platform:   req.Platform,
		Status:     "queued",
		CreatedAt:  time.Now().Unix(),
		OutputPath: outputPath,
	}
	activeJobs.set(job)

	go executeDownloadJob(job, req, info.Account)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobId":   jobID,
	})
}

func executeDownloadJob(job *DownloadJob, req downloadRequestPayload, acc appstore.Account) appstore.Account {
	platform, _ := appstore.ParsePlatform(req.Platform)
	job.Platform = string(platform)
	app := appstore.App{ID: req.AppID}
	outputPath := resolveDownloadOutputPath(req.OutputPath)
	if job.OutputPath == "" {
		job.OutputPath = outputPath
	}

	if req.BundleID != "" {
		lookupResult, err := dependencies.AppStore.Lookup(appstore.LookupInput{
			Account:  acc,
			BundleID: req.BundleID,
			Platform: platform,
		})
		if err == nil {
			app = lookupResult.App
			job.AppName = app.Name
		}
	} else if req.AppID > 0 && req.AppName == "" {
		// Lookup by AppID if no BundleID and no AppName provided
		lookupResult, err := dependencies.AppStore.Lookup(appstore.LookupInput{
			Account:  acc,
			BundleID: "",
			AppID:    req.AppID,
			Platform: platform,
		})
		if err == nil {
			app = lookupResult.App
			job.AppName = app.Name
		}
	} else if req.AppName != "" {
		// Use provided AppName if no lookup was done
		app.Name = req.AppName
	}

	// Download first and only acquire a license when Apple reports
	// ErrLicenseRequired (handled inside downloadWithRetry when AcquireLicense
	// is true). This avoids an extra Purchase round-trip for apps the account
	// already owns or delisted apps that cannot be purchased again.
	progress := newDownloadProgress()
	job.Status = "downloading"
	job.progress = progress
	activeJobs.set(job)

	refreshed, out, _, err := downloadWithRetry(downloadTaskInput{
		Account:           acc,
		App:               app,
		OutputPath:        outputPath,
		ExternalVersionID: req.ExternalVersionID,
		Platform:          platform,
		Progress:          progress.bar,
		AcquireLicense:    req.Purchase,
		OnPurchase: func() {
			activeJobs.update(job.ID, func(j *DownloadJob) {
				j.Status = "purchasing"
			})
		},
		OnDownload: func() {
			activeJobs.update(job.ID, func(j *DownloadJob) {
				j.Status = "downloading"
			})
		},
	})
	acc = refreshed

	if err != nil {
		job.Status = "error"
		job.Error = err.Error()
		activeJobs.set(job)
		return acc
	}

	// The bar carries the exact byte counts of the finished transfer.
	progress.sample(job)

	// Step 3: Replicate SINF signatures.
	//
	// Apple sometimes serves a package with no DRM signatures at all. Recent
	// ipatool versions treat that as a successful, unmodified rewrite instead
	// of returning an error, so detect it from the download output and surface
	// the warning ourselves. macOS downloads are .pkg archives with embedded
	// MASReceipt and do not use SINF replication when out.Sinfs is empty.
	job.Status = "patching"
	activeJobs.set(job)

	if !(platform == appstore.PlatformMacOS && len(out.Sinfs) == 0) {
		err = dependencies.AppStore.ReplicateSinf(appstore.ReplicateSinfInput{
			Sinfs:       out.Sinfs,
			PackagePath: out.DestinationPath,
		})

		if err == nil && len(out.Sinfs) == 0 {
			// Keep the downloaded file: it is a valid, complete package.
			job.Warning = noSinfWarning
		}

		if err != nil {
			job.Status = "error"
			job.Error = fmt.Sprintf("failed to replicate signature: %v", err)
			activeJobs.set(job)
			return acc
		}
	}

	// Name GUI-managed IPA files from their own Info.plist, after download.
	// Explicit output filenames remain exactly as the user entered them.
	if destination, metadata, nameErr := renameDownloadedIPAPackage(
		out.DestinationPath,
		outputPath,
		app.Name,
		acc.Email,
		platform,
	); nameErr != nil {
		job.Warning = appendDownloadJobWarning(job.Warning, downloadFilenameFallbackWarning)
	} else {
		out.DestinationPath = destination
		if metadata.DisplayName != "" {
			job.AppName = metadata.DisplayName
		}
		job.Version = metadata.Version
	}

	// Step 4: Finished!
	job.Status = "completed"
	job.Progress = 100.0
	job.OutputPath = out.DestinationPath
	activeJobs.set(job)

	return acc
}

func handleAPIDownloadStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		jsonError(w, http.StatusBadRequest, "jobId is required")
		return
	}

	job, found := activeJobs.get(jobID)
	if !found {
		jsonError(w, http.StatusNotFound, "job not found")
		return
	}

	jsonResponse(w, http.StatusOK, job)
}

// handleAPIActiveDownloads returns every non-terminal download job (single or
// batch), newest first, so the Downloads tab can render real active jobs even
// after a page reload or for jobs started from other tabs.
func handleAPIActiveDownloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	activeJobs.Lock()
	jobs := make([]*DownloadJob, 0, len(activeJobs.jobs))
	for _, job := range activeJobs.jobs {
		if job.Status == "queued" || job.Status == "purchasing" || job.Status == "downloading" || job.Status == "patching" {
			if job.progress != nil {
				job.progress.sample(job)
			}
			copyJob := *job
			jobs = append(jobs, &copyJob)
		}
	}
	activeJobs.Unlock()

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt > jobs[j].CreatedAt
	})

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobs":    jobs,
	})
}

type iCloudStatus struct {
	Installed   bool   `json:"installed"`
	Variant     string `json:"variant"`
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
}

func handleAPIICloudStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := iCloudStatus{
		Installed:   true,
		Variant:     "built-in",
		DownloadURL: "https://updates.cdn-apple.com/2020/windows/001-39935-20200911-1A70AA56-F448-11EA-8CC0-99D41950005E/iCloudSetup.exe",
	}
	jsonResponse(w, http.StatusOK, status)
}

func handleAPIVersions(w http.ResponseWriter, r *http.Request) {
	bundleID := r.URL.Query().Get("bundleId")
	appIDStr := r.URL.Query().Get("appId")
	platform, _ := appstore.ParsePlatform(r.URL.Query().Get("platform"))
	var appID int64
	if appIDStr != "" {
		appID, _ = strconv.ParseInt(appIDStr, 10, 64)
	}

	if bundleID == "" && appID == 0 {
		jsonError(w, http.StatusBadRequest, "bundleId or appId is required")
		return
	}

	info, err := dependencies.AppStore.AccountInfo()
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "you must log in to list versions")
		return
	}

	app := appstore.App{ID: appID, BundleID: bundleID}
	if appID == 0 && bundleID != "" {
		lookupResult, err := dependencies.AppStore.Lookup(appstore.LookupInput{Account: info.Account, BundleID: bundleID, Platform: platform})
		if err != nil {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("lookup failed: %v", err))
			return
		}
		app = lookupResult.App
	}

	out, err := dependencies.AppStore.ListVersions(appstore.ListVersionsInput{
		Account:  info.Account,
		App:      app,
		Platform: platform,
	})
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":                    true,
		"appID":                      app.ID,
		"name":                       app.Name,
		"bundleID":                   app.BundleID,
		"externalVersionIdentifiers": out.ExternalVersionIdentifiers,
	})
}

func handleAPIVersionMetadata(w http.ResponseWriter, r *http.Request) {
	bundleID := r.URL.Query().Get("bundleId")
	appIDStr := r.URL.Query().Get("appId")
	versionID := r.URL.Query().Get("versionId")
	platform, _ := appstore.ParsePlatform(r.URL.Query().Get("platform"))

	var appID int64
	if appIDStr != "" {
		appID, _ = strconv.ParseInt(appIDStr, 10, 64)
	}

	if versionID == "" {
		jsonError(w, http.StatusBadRequest, "versionId is required")
		return
	}

	result, err := lookupVersionMetadataWithAccount(nil, bundleID, appID, versionID, platform)
	if err != nil {
		if errors.Is(err, errNotAuthenticated) {
			jsonError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, result)
}

// versionMetadataBatchRequest is the body of POST /api/version-metadata/batch.
type versionMetadataBatchRequest struct {
	BundleID   string   `json:"bundleId"`
	AppID      int64    `json:"appId"`
	Platform   string   `json:"platform"`
	VersionIDs []string `json:"versionIds"`
}

// Upper bound on how many builds one batch request may ask for, and on how
// many Apple round-trips run at the same time for it. Apple answers each
// metadata request in ~25 s regardless of parallelism, so fanning several
// out at once turns N sequential waits into one. 50 at a time proved too
// aggressive: roughly a third of the uncached requests came back HTTP 502.
// 10 keeps the fan-out well below that while still cutting a 50-row page
// down to five round-trips.
const versionMetadataBatchLimit = 10

// handleAPIVersionMetadataBatch resolves display version, release date and
// minimum OS version for many builds in a single HTTP round-trip. Browsers cap
// concurrent requests per host at ~6, so one request per build could never
// exceed that parallelism; here the fan-out happens server-side instead.
func handleAPIVersionMetadataBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req versionMetadataBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.VersionIDs) == 0 {
		jsonError(w, http.StatusBadRequest, "versionIds is required")
		return
	}

	if len(req.VersionIDs) > versionMetadataBatchLimit {
		req.VersionIDs = req.VersionIDs[:versionMetadataBatchLimit]
	}

	platform, _ := appstore.ParsePlatform(req.Platform)

	// Resolve account and bundle ID once for the whole batch instead of per build.
	var account *appstore.Account
	appID := req.AppID
	if appID == 0 && req.BundleID != "" {
		info, err := dependencies.AppStore.AccountInfo()
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		account = &info.Account
		if lookupResult, err := dependencies.AppStore.Lookup(appstore.LookupInput{Account: info.Account, BundleID: req.BundleID, Platform: platform}); err == nil {
			appID = lookupResult.App.ID
		}
	}

	results := make(map[string]interface{}, len(req.VersionIDs))
	uncached := make([]string, 0, len(req.VersionIDs))

	if appID != 0 {
		cacheKey := versionMetaCacheKey(appID, platform)
		versionMetaCache.RLock()
		appCache := versionMetaCache.m[cacheKey]
		for _, versionID := range req.VersionIDs {
			if versionID == "" {
				continue
			}
			if entry, ok := appCache[versionID]; ok {
				results[versionID] = map[string]interface{}{
					"success":          true,
					"displayVersion":   entry.DisplayVersion,
					"releaseDate":      entry.ReleaseDate,
					"minimumOSVersion": entry.MinimumOSVersion,
					"cached":           true,
				}
			} else {
				uncached = append(uncached, versionID)
			}
		}
		versionMetaCache.RUnlock()
	} else {
		for _, versionID := range req.VersionIDs {
			if versionID != "" {
				uncached = append(uncached, versionID)
			}
		}
	}

	if len(uncached) > 0 && account == nil {
		info, err := dependencies.AppStore.AccountInfo()
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		account = &info.Account
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	started := time.Now()
	for _, versionID := range uncached {
		wg.Add(1)
		go func(versionID string) {
			defer wg.Done()
			result, err := lookupVersionMetadataWithAccount(account, req.BundleID, appID, versionID, platform)
			if err != nil && isTransientAppleError(err) {
				// One retry for gateway-style failures (502/503/504) that
				// Apple emits under load; a second attempt usually succeeds.
				result, err = lookupVersionMetadataWithAccount(account, req.BundleID, appID, versionID, platform)
			}
			if err != nil {
				result = map[string]interface{}{"success": false, "message": err.Error()}
			}
			mu.Lock()
			results[versionID] = result
			mu.Unlock()
		}(versionID)
	}
	wg.Wait()

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"results": results,
		"totalMs": time.Since(started).Milliseconds(),
	})
}

var errNotAuthenticated = errors.New("not authenticated")

// isTransientAppleError reports whether err looks like a gateway failure
// (HTTP 502/503/504) from Apple rather than a real rejection.
func isTransientAppleError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "(HTTP 502)") ||
		strings.Contains(msg, "(HTTP 503)") ||
		strings.Contains(msg, "(HTTP 504)")
}

// lookupVersionMetadata returns the JSON payload for one build, served from
// versionMetaCache when possible and fetched from Apple otherwise.
func lookupVersionMetadata(bundleID string, appID int64, versionID string) (map[string]interface{}, error) {
	return lookupVersionMetadataWithAccount(nil, bundleID, appID, versionID, "")
}

func lookupVersionMetadataWithAccount(account *appstore.Account, bundleID string, appID int64, versionID string, platform appstore.Platform) (map[string]interface{}, error) {
	// Serve from cache when available (keyed by platform+appID:versionID).
	if appID != 0 {
		cacheKey := versionMetaCacheKey(appID, platform)
		versionMetaCache.RLock()
		entry, ok := versionMetaCache.m[cacheKey][versionID]
		versionMetaCache.RUnlock()
		if ok {
			return map[string]interface{}{
				"success":          true,
				"displayVersion":   entry.DisplayVersion,
				"releaseDate":      entry.ReleaseDate,
				"minimumOSVersion": entry.MinimumOSVersion,
				"cached":           true,
			}, nil
		}
	}

	var acc appstore.Account
	if account != nil {
		acc = *account
	} else {
		info, err := dependencies.AppStore.AccountInfo()
		if err != nil {
			return nil, errNotAuthenticated
		}
		acc = info.Account
	}

	app := appstore.App{ID: appID, BundleID: bundleID}
	// Only resolve the bundle ID into an app ID when the caller did not already
	// supply an app ID; this avoids an extra lookup for every build.
	if bundleID != "" && appID == 0 {
		lookupResult, err := dependencies.AppStore.Lookup(appstore.LookupInput{Account: acc, BundleID: bundleID, Platform: platform})
		if err == nil {
			app = lookupResult.App
			appID = app.ID
		}
	}

	// Time the round-trip to Apple so slow responses can be told apart from
	// slow rendering when investigating "history loads slowly" reports.
	started := time.Now()
	out, err := dependencies.AppStore.GetVersionMetadata(appstore.GetVersionMetadataInput{
		Account:   acc,
		App:       app,
		VersionID: versionID,
		Platform:  platform,
	})
	elapsedMs := time.Since(started).Milliseconds()
	if err != nil {
		return nil, err
	}

	releaseDate := out.ReleaseDate.Format("2006-01-02")

	if appID != 0 {
		cacheKey := versionMetaCacheKey(appID, platform)
		versionMetaCache.Lock()
		if versionMetaCache.m[cacheKey] == nil {
			versionMetaCache.m[cacheKey] = map[string]versionMetaEntry{}
		}
		versionMetaCache.m[cacheKey][versionID] = versionMetaEntry{
			DisplayVersion:   out.DisplayVersion,
			ReleaseDate:      releaseDate,
			MinimumOSVersion: out.MinimumOSVersion,
		}
		versionMetaCache.Unlock()
	}

	return map[string]interface{}{
		"success":          true,
		"displayVersion":   out.DisplayVersion,
		"releaseDate":      releaseDate,
		"minimumOSVersion": out.MinimumOSVersion,
		"appleMs":          elapsedMs,
	}, nil
}

// appIDKey converts an app ID to its cache-map key.
func appIDKey(id int64) string {
	return strconv.FormatInt(id, 10)
}

func versionMetaCacheKey(id int64, platform appstore.Platform) string {
	if platform == "" || platform == appstore.PlatformIPhone {
		return appIDKey(id)
	}
	return string(platform) + ":" + appIDKey(id)
}

type openFolderPayload struct {
	Path string `json:"path"`
}

func handleAPIOpenFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req openFolderPayload
	_ = json.NewDecoder(r.Body).Decode(&req)

	targetPath := strings.TrimSpace(req.Path)
	if targetPath == "" {
		targetPath = defaultDownloadDir()
	} else {
		// If path points to a file, get parent dir
		fi, err := os.Stat(targetPath)
		if err == nil && !fi.IsDir() {
			targetPath = filepath.Dir(targetPath)
		} else if os.IsNotExist(err) && filepath.Ext(targetPath) == "" {
			_ = os.MkdirAll(targetPath, 0o755)
		}
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", targetPath)
	case "darwin":
		cmd = exec.Command("open", targetPath)
	default:
		cmd = exec.Command("xdg-open", targetPath)
	}

	if err := cmd.Start(); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("failed to open folder: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

type saveListPayload struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

func currentWorkingDir() string {
	dir := appWorkingDir()
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func sanitizeTxtFilename(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "purchases.txt"
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r < 32, r == '<', r == '>', r == ':', r == '"', r == '/', r == '\\', r == '|', r == '?', r == '*':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" {
		cleaned = "purchases.txt"
	}
	if !strings.HasSuffix(strings.ToLower(cleaned), ".txt") {
		cleaned += ".txt"
	}
	return cleaned
}

func handleAPISaveList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req saveListPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		jsonError(w, http.StatusBadRequest, "content is empty")
		return
	}

	filename := sanitizeTxtFilename(req.Filename)
	fullPath := filepath.Join(currentWorkingDir(), filename)
	if err := os.WriteFile(fullPath, []byte(req.Content), 0o644); err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save file: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"filename": filename,
		"path":     fullPath,
	})
}

