package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
	pkghttp "github.com/majd/ipatool/v2/pkg/http"
)

const (
	commercePurchasesPath      = "/commerce/account/purchases"
	commerceConfiguratorUA     = "Configurator/2.17 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6"
	commerceAppStoreLaunchYear = 2008
	commerceMaxPagesPerYear    = 50
	commerceMaxRedirectHops    = 5
	commerceYearConcurrency    = 2
	commerceCatalogBatchSize   = 100
	tvosLookupConcurrency      = 4
	purchasesCacheFilename     = "purchases_cache.json"
)

// guiPersistentCookieJar wraps juju/persistent-cookiejar so Apple session
// cookies (wosid, mz_at0-*, mz_at_ssl-*, itspod, ns-mzf-inst, myacinfo) that
// arrive without an explicit Expires/Max-Age are promoted to persistent
// cookies and written to ~/.ipatool/cookies on Save(). Without this,
// persistent-cookiejar drops all session cookies when the process exits,
// causing /commerce/account/purchases to lose its session on the next run.
type guiPersistentCookieJar struct {
	inner pkghttp.CookieJar
}

type netscapeCookieEntry struct {
	u      *url.URL
	cookie *http.Cookie
}

func newGUICookieJar(stateDirectory string) pkghttp.CookieJar {
	cookiePath := filepath.Join(stateDirectory, CookieJarFileName)
	imported := importNetscapeCookieFileIfNeeded(cookiePath)

	baseJar := newCookieJar(stateDirectory)
	wrapped := &guiPersistentCookieJar{inner: baseJar}

	if len(imported) > 0 {
		for _, entry := range imported {
			wrapped.SetCookies(entry.u, []*http.Cookie{entry.cookie})
		}
		_ = wrapped.Save()
	}

	return wrapped
}

func importNetscapeCookieFileIfNeeded(cookiePath string) []netscapeCookieEntry {
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return nil
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		_ = os.Remove(cookiePath)
		return nil
	}

	// juju/persistent-cookiejar stores a JSON array starting with '['.
	if trimmed[0] == '[' {
		return nil
	}

	var entries []netscapeCookieEntry
	lines := strings.Split(string(trimmed), "\n")
	defaultExpiry := time.Now().Add(14 * 24 * time.Hour)

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		httpOnly := false
		if strings.HasPrefix(line, "#HttpOnly_") {
			httpOnly = true
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}

		domain := strings.TrimSpace(fields[0])
		path := strings.TrimSpace(fields[2])
		secure := strings.EqualFold(strings.TrimSpace(fields[3]), "TRUE")
		expSec, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		name := strings.TrimSpace(fields[5])
		value := fields[6]

		if domain == "" || name == "" {
			continue
		}
		if path == "" {
			path = "/"
		}

		host := strings.TrimPrefix(domain, ".")
		scheme := "https"
		if !secure {
			scheme = "http"
		}
		u, parseErr := url.Parse(fmt.Sprintf("%s://%s%s", scheme, host, path))
		if parseErr != nil {
			continue
		}

		exp := defaultExpiry
		if expSec > 0 {
			parsedExp := time.Unix(expSec, 0)
			if parsedExp.After(time.Now()) {
				exp = parsedExp
			}
		}

		entries = append(entries, netscapeCookieEntry{
			u: u,
			cookie: &http.Cookie{
				Name:     name,
				Value:    value,
				Domain:   domain,
				Path:     path,
				Secure:   secure,
				HttpOnly: httpOnly,
				Expires:  exp,
			},
		})
	}

	// Remove the non-JSON file before juju/persistent-cookiejar opens it.
	_ = os.Remove(cookiePath)
	return entries
}

func (j *guiPersistentCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if len(cookies) == 0 {
		return
	}
	promoted := make([]*http.Cookie, 0, len(cookies))
	defaultExpiry := time.Now().Add(14 * 24 * time.Hour)
	for _, c := range cookies {
		if c == nil {
			continue
		}
		cp := *c
		if cp.MaxAge == 0 && cp.Expires.IsZero() {
			cp.Expires = defaultExpiry
		}
		promoted = append(promoted, &cp)
	}
	j.inner.SetCookies(u, promoted)
}

func (j *guiPersistentCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.inner.Cookies(u)
}

func (j *guiPersistentCookieJar) Save() error {
	return j.inner.Save()
}

// purchasesCache keeps the last purchase-history result per Apple ID so the
// "Purchase history" tab is populated instantly after the first load. The
// client asks for a fresh copy explicitly with ?refresh=1.
type purchasesCache struct {
	sync.Mutex
	dsid        string
	apps        []appstore.App
	hiddenIDs   map[int64]bool
	tvosVerByID map[int64]string
	fetchedAt   time.Time
	// inflight collapses concurrent loads (e.g. a refresh click while the
	// initial load is still running) into one Apple round-trip.
	inflight *purchasesLoad
}

type purchasesLoad struct {
	done        chan struct{}
	apps        []appstore.App
	hiddenIDs   map[int64]bool
	tvosVerByID map[int64]string
	err         error
}

type purchasesDiskCache struct {
	DSID      string         `json:"dsid,omitempty"`
	Email     string         `json:"email,omitempty"`
	FetchedAt int64          `json:"fetchedAt"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
	Items     []purchaseItem `json:"items"`
}

var purchasesState = &purchasesCache{}

var commerceRefreshMu sync.Mutex

// purchaseItem is the JSON shape consumed by the GUI.
type purchaseItem struct {
	AppID                int64               `json:"appId"`
	BundleID             string              `json:"bundleId"`
	Name                 string              `json:"name"`
	Version              string              `json:"version,omitempty"`
	TVOSVersion          string              `json:"tvosVersion,omitempty"`
	PurchaseDate         string              `json:"purchaseDate,omitempty"`
	Platforms            []appstore.Platform `json:"platforms,omitempty"`
	HiddenFromStorefront bool                `json:"hiddenFromStorefront,omitempty"`
}

type commercePurchasesResponse struct {
	Data *struct {
		Attributes *struct {
			IsComplete *bool                     `json:"is-complete"`
			Purchases  []commercePurchaseInvoice `json:"purchases"`
		} `json:"attributes"`
	} `json:"data"`
}

type commercePurchaseInvoice struct {
	OrderID     string                 `json:"order-id"`
	InvoiceDate string                 `json:"invoice-date"`
	Items       []commercePurchaseItem `json:"items"`
}

type commercePurchaseItem struct {
	ItemID       interface{} `json:"item-id"`
	ItemName     string      `json:"item-name"`
	ItemTitle    string      `json:"item-title"`
	Price        string      `json:"price"`
	PurchaseDate string      `json:"purchase-date"`
	Kind         string      `json:"kind"`
}

type catalogEnrichmentItem struct {
	TrackID          int64    `json:"trackId"`
	BundleID         string   `json:"bundleId"`
	Version          string   `json:"version"`
	Name             string   `json:"trackName"`
	Kind             string   `json:"kind"`
	SupportedDevices []string `json:"supportedDevices"`
}

type catalogEnrichmentResponse struct {
	Results []catalogEnrichmentItem `json:"results"`
}

func fetchDAAPOwnedAppsForAccount(acc appstore.Account) ([]appstore.App, error) {
	var allApps []appstore.App
	page := 1
	for {
		out, err := dependencies.AppStore.OwnedApps(appstore.OwnedAppsInput{
			Account: acc,
			Page:    page,
			Limit:   appstore.MaxOwnedAppsLimit,
		})
		if err != nil {
			return nil, err
		}
		allApps = append(allApps, out.Results...)
		if len(out.Results) == 0 || len(allApps) >= out.TotalCount {
			break
		}
		page++
	}
	return allApps, nil
}

// fetchDAAPOwnedApps queries the user's primary regional storefront and, when
// the account is outside the US/GB storefronts, also probes the US storefront
// so purchased apps hidden only in the regional storefront are discovered via
// DAAP as well.
func fetchDAAPOwnedApps(acc appstore.Account) (regionalApps []appstore.App, fallbackApps []appstore.App, err error) {
	regionalApps, err = fetchDAAPOwnedAppsForAccount(acc)
	if err != nil {
		return nil, nil, err
	}

	trimmedSF := strings.TrimSpace(acc.StoreFront)
	if !strings.HasPrefix(trimmedSF, "143441") {
		usAcc := acc
		usAcc.StoreFront = "143441-1,29"
		if usApps, usErr := fetchDAAPOwnedAppsForAccount(usAcc); usErr == nil && len(usApps) > 0 {
			fallbackApps = append(fallbackApps, usApps...)
		}
	}

	return regionalApps, fallbackApps, nil
}

func fetchAllOwnedApps(acc appstore.Account) ([]appstore.App, map[int64]bool, map[int64]string, error) {
	var (
		regionalDAAPApps []appstore.App
		fallbackDAAPApps []appstore.App
		daapErr          error
		commerceApps     []appstore.App
		commercePod      string
		commerceErr      error
		wg               sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		regionalDAAPApps, fallbackDAAPApps, daapErr = fetchDAAPOwnedApps(acc)
	}()
	go func() {
		defer wg.Done()
		commerceApps, commercePod, commerceErr = fetchCommercePurchaseHistoryOnce(acc)
	}()
	wg.Wait()

	if commerceErr == nil {
		persistAccountPodIfChanged(acc, commercePod)
	}

	// If DAAP failed due to an expired password token, let ownedAppsWithRetry
	// perform a single refreshAccount (which refreshes both PasswordToken and
	// web cookies) and retry the whole fetch cleanly.
	if daapErr != nil && errors.Is(daapErr, appstore.ErrPasswordTokenExpired) {
		return nil, nil, nil, daapErr
	}

	// If DAAP succeeded and only the short-lived web cookies for
	// /commerce/account/purchases expired, refresh once now (after DAAP has
	// finished, avoiding any race) and retry commerce. If that refresh fails
	// (e.g. transient fpinit timeout), keep the DAAP apps rather than failing.
	if daapErr == nil && commerceErr != nil && errors.Is(commerceErr, appstore.ErrPasswordTokenExpired) && acc.Password != "" {
		commerceRefreshMu.Lock()
		refreshedAcc, refreshErr := refreshAccount(acc)
		commerceRefreshMu.Unlock()
		if refreshErr == nil {
			if retriedApps, retriedPod, retryErr := fetchCommercePurchaseHistoryOnce(refreshedAcc); retryErr == nil {
				persistAccountPodIfChanged(refreshedAcc, retriedPod)
				commerceApps = retriedApps
				commerceErr = nil
			}
		}
	}

	if daapErr != nil && len(regionalDAAPApps) == 0 {
		if commerceErr == nil && len(commerceApps) > 0 {
			enrichMissingCatalogFields(commerceApps)
			tvosVerByID := enrichTVOSCatalog(acc, commerceApps)
			return sortPurchasesByDate(commerceApps), nil, tvosVerByID, nil
		}
		if errors.Is(commerceErr, appstore.ErrPasswordTokenExpired) {
			return nil, nil, nil, commerceErr
		}
		return nil, nil, nil, daapErr
	}

	daapSeen := make(map[int64]struct{}, len(regionalDAAPApps))
	for _, app := range regionalDAAPApps {
		if app.ID > 0 {
			daapSeen[app.ID] = struct{}{}
		}
	}

	extraApps := mergePurchaseHistoryApps(fallbackDAAPApps, commerceApps)

	hiddenIDs := make(map[int64]bool)
	for _, app := range extraApps {
		if app.ID > 0 {
			if _, ok := daapSeen[app.ID]; !ok {
				hiddenIDs[app.ID] = true
			}
		}
	}

	merged := mergePurchaseHistoryApps(regionalDAAPApps, extraApps)
	enrichMissingCatalogFields(merged)
	tvosVerByID := enrichTVOSCatalog(acc, merged)

	return sortPurchasesByDate(merged), hiddenIDs, tvosVerByID, nil
}

func fetchCommercePurchaseHistoryWithRelogin(acc appstore.Account) ([]appstore.App, error) {
	apps, updatedPod, err := fetchCommercePurchaseHistoryOnce(acc)
	if err == nil {
		persistAccountPodIfChanged(acc, updatedPod)
		return apps, nil
	}

	// If session cookies in ~/.ipatool/cookies were missing or expired,
	// refreshAccount performs a silent Login (populating fresh wosid/mz_at0
	// cookies into dependencies.CookieJar) and we retry immediately.
	if acc.Password != "" {
		commerceRefreshMu.Lock()
		refreshedAcc, refreshErr := refreshAccount(acc)
		commerceRefreshMu.Unlock()
		if refreshErr == nil {
			apps, updatedPod, err = fetchCommercePurchaseHistoryOnce(refreshedAcc)
			if err == nil {
				persistAccountPodIfChanged(refreshedAcc, updatedPod)
				return apps, nil
			}
		}
	}

	return nil, err
}

func persistAccountPodIfChanged(acc appstore.Account, newPod string) {
	newPod = strings.TrimSpace(newPod)
	if newPod == "" || newPod == strings.TrimSpace(acc.Pod) || dependencies.Keychain == nil {
		return
	}
	acc.Pod = newPod
	if data, err := json.Marshal(acc); err == nil {
		_ = dependencies.Keychain.Set("account", data)
	}
}

func newCommerceHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Match sorvigolova/ipatool: do not send Accept-Encoding: gzip so Apple
	// returns plain JSON directly.
	transport.DisableCompression = true

	client := &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if dependencies.CookieJar != nil {
		client.Jar = dependencies.CookieJar
	}
	return client
}

func fetchCommercePurchaseHistoryOnce(acc appstore.Account) ([]appstore.App, string, error) {
	if strings.TrimSpace(acc.DirectoryServicesID) == "" {
		return nil, "", nil
	}

	client := newCommerceHTTPClient()

	podMu := sync.Mutex{}
	activePod := strings.TrimSpace(acc.Pod)
	getPod := func() string {
		podMu.Lock()
		defer podMu.Unlock()
		return activePod
	}
	setPod := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		podMu.Lock()
		activePod = p
		podMu.Unlock()
	}

	// Step 1: Fetch the default 90-day window (range="") first, just like iTunes
	// Purchase History. This also completes the 302 pod/cookie handshake
	// (mzcprh=1 & Set-Cookie itspod=N) and validates that our session cookies
	// are accepted before querying historical years.
	recentApps, status, err := fetchCommercePurchasesForRange(client, acc, getPod, setPod, "", 0)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, getPod(), appstore.ErrPasswordTokenExpired
	}
	if err != nil {
		return nil, getPod(), err
	}

	if dependencies.CookieJar != nil {
		_ = dependencies.CookieJar.Save()
	}

	currentYear := time.Now().UTC().Year()
	if currentYear < commerceAppStoreLaunchYear {
		currentYear = commerceAppStoreLaunchYear
	}

	type yearResult struct {
		year int
		apps []appstore.App
	}

	years := make([]int, 0, currentYear-commerceAppStoreLaunchYear+1)
	for y := currentYear; y >= commerceAppStoreLaunchYear; y-- {
		years = append(years, y)
	}

	results := make([]yearResult, len(years))
	sem := make(chan struct{}, commerceYearConcurrency)
	var wg sync.WaitGroup

	for idx, year := range years {
		wg.Add(1)
		go func(i, y int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			rangeStr := fmt.Sprintf("%d-all", y)
			apps, _, rangeErr := fetchCommercePurchasesForRange(client, acc, getPod, setPod, rangeStr, y)
			if rangeErr != nil {
				// Retry once after a short pause if a concurrent request was throttled.
				time.Sleep(250 * time.Millisecond)
				apps, _, rangeErr = fetchCommercePurchasesForRange(client, acc, getPod, setPod, rangeStr, y)
			}
			if rangeErr == nil && len(apps) > 0 {
				results[i] = yearResult{year: y, apps: apps}
			}
		}(idx, year)
	}

	wg.Wait()

	if dependencies.CookieJar != nil {
		_ = dependencies.CookieJar.Save()
	}

	allApps := make([]appstore.App, 0, len(recentApps)+128)
	allApps = append(allApps, recentApps...)
	for _, res := range results {
		allApps = append(allApps, res.apps...)
	}

	return mergePurchaseHistoryApps(nil, allApps), getPod(), nil
}

func fetchCommercePurchasesForRange(
	client *http.Client,
	acc appstore.Account,
	getPod func() string,
	setPod func(string),
	rangeParam string,
	fallbackYear int,
) ([]appstore.App, int, error) {
	seenOrders := make(map[string]struct{})
	var apps []appstore.App

	for page := 1; page <= commerceMaxPagesPerYear; page++ {
		body, status, err := sendCommercePurchasesRequest(client, acc, getPod(), setPod, page, rangeParam)
		if err != nil {
			return apps, status, err
		}
		if status != http.StatusOK {
			return apps, status, fmt.Errorf("commerce purchase history returned HTTP %d", status)
		}

		pageApps, isComplete, addedInvoices, ok := parseCommercePurchasesPage(body, seenOrders)
		if !ok {
			return apps, status, errors.New("unexpected commerce purchase history response")
		}

		if fallbackYear > 0 {
			fallbackDate := time.Date(fallbackYear, time.January, 1, 0, 0, 0, 0, time.UTC)
			for i := range pageApps {
				if pageApps[i].PurchaseDate.IsZero() {
					pageApps[i].PurchaseDate = fallbackDate
				}
			}
		}

		apps = append(apps, pageApps...)
		if isComplete || addedInvoices == 0 {
			break
		}
	}

	return apps, http.StatusOK, nil
}

func sendCommercePurchasesRequest(
	client *http.Client,
	acc appstore.Account,
	pod string,
	setPod func(string),
	page int,
	rangeParam string,
) ([]byte, int, error) {
	buildURL := func(targetPod string) string {
		prefix := ""
		if targetPod != "" {
			prefix = "p" + targetPod + "-"
		}
		query := "isDeepLink=false&isJsonApiFormat=true"
		if page > 0 {
			query += "&page=" + strconv.Itoa(page)
		}
		if rangeParam != "" {
			query += "&range=" + url.QueryEscape(rangeParam)
		}
		return fmt.Sprintf("https://%sbuy.itunes.apple.com%s?%s", prefix, commercePurchasesPath, query)
	}

	storeFrontHeader := strings.TrimSpace(acc.StoreFront)

	doGET := func(rawURL string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", commerceConfiguratorUA)
		req.Header.Set("Accept-Language", "en-us")
		req.Header.Set("X-Apple-Store-Front", storeFrontHeader)
		req.Header.Set("X-Dsid", acc.DirectoryServicesID)
		req.Header.Set("iCloud-DSID", acc.DirectoryServicesID)
		if acc.PasswordToken != "" {
			req.Header.Set("X-Token", acc.PasswordToken)
		}
		return client.Do(req)
	}

	targetURL := buildURL(pod)
	res, err := doGET(targetURL)
	if err != nil && pod != "" {
		// Stale pod hostname: retry on the pod-less host and let the 302 redirect route us.
		res, err = doGET(buildURL(""))
	}
	if err != nil {
		return nil, 0, err
	}

	for hop := 0; isCommerceRedirectStatus(res.StatusCode) && hop < commerceMaxRedirectHops; hop++ {
		if headerPod := strings.TrimSpace(res.Header.Get("pod")); headerPod != "" {
			setPod(headerPod)
		}
		loc := strings.TrimSpace(res.Header.Get("Location"))
		reqURL := res.Request.URL
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if loc == "" {
			return nil, res.StatusCode, errors.New("commerce purchase history redirect missing Location")
		}

		nextURL := loc
		if reqURL != nil {
			if parsedLoc, parseErr := reqURL.Parse(loc); parseErr == nil {
				nextURL = parsedLoc.String()
			}
		}

		if locPod := extractPodFromURL(nextURL); locPod != "" {
			setPod(locPod)
		}
		res, err = doGET(nextURL)
		if err != nil {
			return nil, 0, err
		}
	}

	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	if headerPod := strings.TrimSpace(res.Header.Get("pod")); headerPod != "" {
		setPod(headerPod)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.StatusCode, err
	}

	return body, res.StatusCode, nil
}

func isCommerceRedirectStatus(code int) bool {
	switch code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func extractPodFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if !strings.HasPrefix(host, "p") || !strings.HasSuffix(host, "-buy.itunes.apple.com") {
		return ""
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(host, "p"), "-buy.itunes.apple.com")
	if digits == "" {
		return ""
	}
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return digits
}

func parseCommercePurchasesPage(rawBody []byte, seenOrders map[string]struct{}) ([]appstore.App, bool, int, bool) {
	var payload commercePurchasesResponse
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, true, 0, false
	}
	if payload.Data == nil || payload.Data.Attributes == nil {
		return nil, true, 0, false
	}

	attr := payload.Data.Attributes
	isComplete := true
	if attr.IsComplete != nil {
		isComplete = *attr.IsComplete
	}

	addedInvoices := 0
	var apps []appstore.App

	for _, inv := range attr.Purchases {
		orderID := strings.TrimSpace(inv.OrderID)
		if orderID != "" {
			if _, exists := seenOrders[orderID]; exists {
				continue
			}
			seenOrders[orderID] = struct{}{}
		}
		addedInvoices++

		invDate := strings.TrimSpace(inv.InvoiceDate)
		for _, item := range inv.Items {
			appID := parseCommerceItemID(item.ItemID)
			if appID <= 0 {
				continue
			}

			platforms, ok := platformsFromCommerceKind(item.Kind)
			if !ok {
				continue
			}

			name := strings.TrimSpace(item.ItemName)
			if name == "" {
				name = strings.TrimSpace(item.ItemTitle)
			}

			dateStr := strings.TrimSpace(item.PurchaseDate)
			if dateStr == "" {
				dateStr = invDate
			}

			apps = append(apps, appstore.App{
				ID:           appID,
				Name:         name,
				PurchaseDate: parseCommerceDate(dateStr),
				Platforms:    platforms,
			})
		}
	}

	return apps, isComplete, addedInvoices, true
}

func parseCommerceItemID(raw interface{}) int64 {
	switch v := raw.(type) {
	case string:
		id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil && id > 0 {
			return id
		}
	case float64:
		if v > 0 {
			return int64(v)
		}
	case json.Number:
		id, err := v.Int64()
		if err == nil && id > 0 {
			return id
		}
	}
	return 0
}

func platformsFromCommerceKind(kind string) ([]appstore.Platform, bool) {
	lower := strings.ToLower(strings.TrimSpace(kind))
	if lower == "" {
		return []appstore.Platform{appstore.PlatformIPhone}, true
	}

	for _, nonApp := range []string{
		"audiobook", "ebook", "book", "song", "music", "album", "movie",
		"ringtone", "podcast", "tv episode", "tv season",
		"icloud+", "icloud storage", "subscription", "in-app", "in app",
		"подписк", "встроенн", "книг", "аудиокниг", "песн", "альбом", "фильм", "рингтон",
	} {
		if strings.Contains(lower, nonApp) {
			return nil, false
		}
	}

	switch {
	case strings.Contains(lower, "mac"):
		return []appstore.Platform{appstore.PlatformMacOS}, true
	case strings.Contains(lower, "vision"):
		return []appstore.Platform{appstore.PlatformVisionOS}, true
	case strings.Contains(lower, "watch"):
		return []appstore.Platform{appstore.PlatformWatchOS}, true
	case strings.Contains(lower, "tv"):
		return []appstore.Platform{appstore.PlatformAppleTV}, true
	case strings.Contains(lower, "universal") || strings.Contains(lower, "универсал"):
		return []appstore.Platform{appstore.PlatformIPhone, appstore.PlatformIPad}, true
	case strings.Contains(lower, "ipad"):
		return []appstore.Platform{appstore.PlatformIPad}, true
	default:
		return []appstore.Platform{appstore.PlatformIPhone}, true
	}
}

var russianMonthsReplacer = strings.NewReplacer(
	"января", "Jan", "февраля", "Feb", "марта", "Mar", "апреля", "Apr",
	"мая", "May", "июня", "Jun", "июля", "Jul", "августа", "Aug",
	"сентября", "Sep", "октября", "Oct", "ноября", "Nov", "декабря", "Dec",
	"янв.", "Jan", "февр.", "Feb", "фев.", "Feb", "мар.", "Mar", "апр.", "Apr",
	"июн.", "Jun", "июл.", "Jul", "авг.", "Aug", "сент.", "Sep", "сен.", "Sep",
	"окт.", "Oct", "нояб.", "Nov", "ноя.", "Nov", "дек.", "Dec",
	"янв", "Jan", "фев", "Feb", "мар", "Mar", "апр", "Apr",
	"май", "May", "июн", "Jun", "июл", "Jul", "авг", "Aug",
	"сен", "Sep", "окт", "Oct", "ноя", "Nov", "дек", "Dec",
)

func parseCommerceDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}

	normalized := strings.ToLower(raw)
	normalized = strings.TrimSuffix(normalized, "г.")
	normalized = strings.TrimSuffix(normalized, "г")
	normalized = strings.TrimSpace(russianMonthsReplacer.Replace(normalized))
	normalized = strings.Join(strings.Fields(normalized), " ")

	candidates := []string{raw}
	if normalized != "" && normalized != strings.ToLower(raw) {
		// Title-case English month names after replacement (e.g. "10 may 2021" -> "10 May 2021")
		words := strings.Fields(normalized)
		for i, w := range words {
			if len(w) == 3 {
				words[i] = strings.ToUpper(w[:1]) + w[1:]
			}
		}
		candidates = append(candidates, strings.Join(words, " "))
	}

	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"Jan 2, 2006",
		"January 2, 2006",
		"2 Jan 2006",
		"02 Jan 2006",
		"2 January 2006",
		"02.01.2006",
		"02/01/2006",
		"01/02/2006",
	}

	for _, candidate := range candidates {
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, candidate); err == nil {
				return parsed.UTC()
			}
		}
	}

	return time.Time{}
}

func mergePurchaseHistoryApps(daapApps, commerceApps []appstore.App) []appstore.App {
	merged := make([]appstore.App, 0, len(daapApps)+len(commerceApps))
	seen := make(map[int64]int, len(daapApps)+len(commerceApps))

	addOrMerge := func(app appstore.App) {
		if app.ID <= 0 {
			return
		}
		if idx, exists := seen[app.ID]; exists {
			existing := &merged[idx]
			if existing.Name == "" && app.Name != "" {
				existing.Name = app.Name
			}
			if existing.BundleID == "" && app.BundleID != "" {
				existing.BundleID = app.BundleID
			}
			if existing.Version == "" && app.Version != "" {
				existing.Version = app.Version
			}
			if existing.PurchaseDate.IsZero() || (!app.PurchaseDate.IsZero() && app.PurchaseDate.After(existing.PurchaseDate)) {
				existing.PurchaseDate = app.PurchaseDate
			}
			for _, p := range app.Platforms {
				if p != "" && p != appstore.PlatformUnknown && !slices.Contains(existing.Platforms, p) {
					existing.Platforms = append(existing.Platforms, p)
				}
			}
			if len(existing.Platforms) > 1 {
				existing.Platforms = slices.DeleteFunc(existing.Platforms, func(p appstore.Platform) bool {
					return p == appstore.PlatformUnknown
				})
			}
			return
		}

		seen[app.ID] = len(merged)
		merged = append(merged, app)
	}

	for _, app := range daapApps {
		addOrMerge(app)
	}
	for _, app := range commerceApps {
		addOrMerge(app)
	}

	return merged
}

// enrichMissingCatalogFields populates BundleID, Version, and Name for apps
// that came only from /commerce/account/purchases (which does not include
// bundle identifiers). It checks the bundled removed-apps catalog first and
// then performs batch iTunes Lookup requests across the default storefront
// plus US/GB fallback storefronts.
func enrichMissingCatalogFields(apps []appstore.App) {
	if len(apps) == 0 {
		return
	}

	removedByID := make(map[int64]removedAppEntry)
	for _, entry := range loadRemovedApps() {
		if entry.AppID > 0 {
			removedByID[entry.AppID] = entry
		}
	}

	missingIndices := make(map[int64]int)
	missingIDs := make([]int64, 0)

	for idx := range apps {
		if entry, ok := removedByID[apps[idx].ID]; ok {
			if apps[idx].Name == "" && entry.Name != "" {
				apps[idx].Name = entry.Name
			}
		}
		if apps[idx].ID > 0 && (apps[idx].BundleID == "" || apps[idx].Version == "") {
			if _, exists := missingIndices[apps[idx].ID]; !exists {
				missingIndices[apps[idx].ID] = idx
				missingIDs = append(missingIDs, apps[idx].ID)
			}
		}
	}

	if len(missingIDs) == 0 {
		return
	}

	for start := 0; start < len(missingIDs); start += commerceCatalogBatchSize {
		end := start + commerceCatalogBatchSize
		if end > len(missingIDs) {
			end = len(missingIDs)
		}

		remaining := append([]int64(nil), missingIDs[start:end]...)
		for _, country := range []string{"", "us", "gb"} {
			if len(remaining) == 0 {
				break
			}

			strIDs := make([]string, len(remaining))
			for i, id := range remaining {
				strIDs[i] = strconv.FormatInt(id, 10)
			}

			items, err := lookupCatalogEnrichment(strIDs, country)
			if err != nil {
				continue
			}

			found := make(map[int64]struct{}, len(items))
			for _, entry := range items {
				if entry.TrackID == 0 {
					continue
				}
				found[entry.TrackID] = struct{}{}
				if idx, ok := missingIndices[entry.TrackID]; ok {
					if apps[idx].BundleID == "" && entry.BundleID != "" {
						apps[idx].BundleID = entry.BundleID
					}
					if apps[idx].Version == "" && entry.Version != "" {
						apps[idx].Version = entry.Version
					}
					if apps[idx].Name == "" && entry.Name != "" {
						apps[idx].Name = entry.Name
					}
					for _, p := range platformsFromCatalogEnrichment(entry) {
						if !slices.Contains(apps[idx].Platforms, p) {
							apps[idx].Platforms = append(apps[idx].Platforms, p)
						}
					}
				}
			}

			nextRemaining := make([]int64, 0, len(remaining))
			for _, id := range remaining {
				if _, ok := found[id]; !ok {
					nextRemaining = append(nextRemaining, id)
				}
			}
			remaining = nextRemaining
		}
	}
}

func platformsFromCatalogEnrichment(entry catalogEnrichmentItem) []appstore.Platform {
	var platforms []appstore.Platform
	for _, family := range []struct {
		prefix   string
		platform appstore.Platform
	}{
		{"iPhone", appstore.PlatformIPhone},
		{"iPod", appstore.PlatformIPhone},
		{"iPad", appstore.PlatformIPad},
		{"AppleTV", appstore.PlatformAppleTV},
		{"Watch", appstore.PlatformWatchOS},
		{"RealityDevice", appstore.PlatformVisionOS},
		{"Mac", appstore.PlatformMacOS},
	} {
		for _, dev := range entry.SupportedDevices {
			if strings.HasPrefix(dev, family.prefix) && !slices.Contains(platforms, family.platform) {
				platforms = append(platforms, family.platform)
			}
		}
	}
	if entry.Kind == "mac-software" && !slices.Contains(platforms, appstore.PlatformMacOS) {
		platforms = append(platforms, appstore.PlatformMacOS)
	}
	if entry.Kind == "tv-software" && !slices.Contains(platforms, appstore.PlatformAppleTV) {
		platforms = append(platforms, appstore.PlatformAppleTV)
	}
	return platforms
}

func storefrontCountryOrDefault(storeFront string) string {
	trimmed := strings.TrimSpace(storeFront)
	if idx := strings.IndexAny(trimmed, "-, "); idx > 0 {
		trimmed = trimmed[:idx]
	}
	switch trimmed {
	case "143469":
		return "ru"
	case "143441":
		return "us"
	case "143444":
		return "gb"
	case "143517":
		return "kz"
	case "143565":
		return "by"
	case "143524":
		return "am"
	case "143566":
		return "uz"
	default:
		return "ru"
	}
}

// enrichTVOSCatalog performs batched iTunes Lookup queries with entity=tvSoftware
// across the account's storefront and fallback storefronts so owned apps that
// have an Apple TV release (such as KION or Okko) receive the tvOS platform
// badge and their tvOS-specific version string.
func enrichTVOSCatalog(acc appstore.Account, apps []appstore.App) map[int64]string {
	tvosVerByID := make(map[int64]string)
	if len(apps) == 0 {
		return tvosVerByID
	}

	idxByID := make(map[int64]int, len(apps))
	allIDs := make([]int64, 0, len(apps))
	for idx, app := range apps {
		if app.ID > 0 {
			if _, exists := idxByID[app.ID]; !exists {
				idxByID[app.ID] = idx
				allIDs = append(allIDs, app.ID)
			}
		}
	}
	if len(allIDs) == 0 {
		return tvosVerByID
	}

	primaryCC := storefrontCountryOrDefault(acc.StoreFront)
	countries := []string{primaryCC}
	for _, fallback := range []string{"ru", "us"} {
		if !slices.Contains(countries, fallback) {
			countries = append(countries, fallback)
		}
	}

	type batchRange struct {
		start int
		end   int
	}
	var batches []batchRange
	for start := 0; start < len(allIDs); start += commerceCatalogBatchSize {
		end := start + commerceCatalogBatchSize
		if end > len(allIDs) {
			end = len(allIDs)
		}
		batches = append(batches, batchRange{start: start, end: end})
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, tvosLookupConcurrency)
	)

	for _, b := range batches {
		wg.Add(1)
		go func(ids []int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			strIDs := make([]string, len(ids))
			for i, id := range ids {
				strIDs[i] = strconv.FormatInt(id, 10)
			}

			for _, country := range countries {
				items, err := lookupCatalogWithEntity(strIDs, country, "tvSoftware")
				if err != nil || len(items) == 0 {
					continue
				}

				mu.Lock()
				for _, entry := range items {
					if entry.TrackID == 0 {
						continue
					}
					idx, ok := idxByID[entry.TrackID]
					if !ok {
						continue
					}
					if !slices.Contains(apps[idx].Platforms, appstore.PlatformAppleTV) {
						apps[idx].Platforms = append(apps[idx].Platforms, appstore.PlatformAppleTV)
					}
					if len(apps[idx].Platforms) > 1 {
						apps[idx].Platforms = slices.DeleteFunc(apps[idx].Platforms, func(p appstore.Platform) bool {
							return p == appstore.PlatformUnknown
						})
					}
					if entry.Version != "" && tvosVerByID[entry.TrackID] == "" {
						tvosVerByID[entry.TrackID] = entry.Version
					}
					if apps[idx].BundleID == "" && entry.BundleID != "" {
						apps[idx].BundleID = entry.BundleID
					}
					if apps[idx].Name == "" && entry.Name != "" {
						apps[idx].Name = entry.Name
					}
				}
				mu.Unlock()
			}
		}(allIDs[b.start:b.end])
	}

	wg.Wait()
	return tvosVerByID
}

func lookupCatalogEnrichment(ids []string, country string) ([]catalogEnrichmentItem, error) {
	return lookupCatalogWithEntity(ids, country, "software,iPadSoftware,macSoftware")
}

func lookupCatalogWithEntity(ids []string, country, entity string) ([]catalogEnrichmentItem, error) {
	query := url.Values{}
	query.Set("id", strings.Join(ids, ","))
	query.Set("entity", entity)
	if country != "" {
		query.Set("country", country)
	}

	res, err := displayLookupClient.Get(displayLookupURL + "?" + query.Encode())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errDisplayLookupFailed
	}

	var payload catalogEnrichmentResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return payload.Results, nil
}

func sortPurchasesByDate(apps []appstore.App) []appstore.App {
	sorted := append([]appstore.App(nil), apps...)
	sort.SliceStable(sorted, func(left, right int) bool {
		leftDate, rightDate := sorted[left].PurchaseDate, sorted[right].PurchaseDate
		if leftDate.IsZero() && rightDate.IsZero() {
			return sorted[left].ID > sorted[right].ID
		}
		if leftDate.IsZero() {
			return false
		}
		if rightDate.IsZero() {
			return true
		}
		return leftDate.After(rightDate)
	})
	return sorted
}

// ownedAppsWithRetry loads the full purchase history and refreshes the stored
// session once when Apple reports an expired password token.
//
//nolint:wrapcheck
func ownedAppsWithRetry(acc appstore.Account) (appstore.Account, []appstore.App, map[int64]bool, map[int64]string, error) {
	refreshed := false

	for attempt := 0; attempt < 2; attempt++ {
		apps, hiddenIDs, tvosVerByID, err := fetchAllOwnedApps(acc)
		if err == nil {
			return acc, apps, hiddenIDs, tvosVerByID, nil
		}

		if errors.Is(err, appstore.ErrPasswordTokenExpired) && !refreshed {
			if acc.Password == "" {
				return acc, nil, nil, nil, errors.New("password token is expired and no password is stored; log in again to refresh the session")
			}

			refreshedAcc, refreshErr := refreshAccount(acc)
			if refreshErr != nil {
				return acc, nil, nil, nil, refreshErr
			}

			acc = refreshedAcc
			refreshed = true
			continue
		}

		return acc, nil, nil, nil, err
	}

	return acc, nil, nil, nil, errors.New("failed to load purchase history after re-authentication")
}

func purchasesCacheFilePath() string {
	if dir := appWorkingDir(); dir != "" {
		return filepath.Join(dir, purchasesCacheFilename)
	}
	return purchasesCacheFilename
}

func buildPurchaseItems(apps []appstore.App, hiddenIDs map[int64]bool, tvosVerByID map[int64]string) []purchaseItem {
	items := make([]purchaseItem, 0, len(apps))
	for _, app := range apps {
		purchaseDate := ""
		if !app.PurchaseDate.IsZero() {
			purchaseDate = app.PurchaseDate.Format("2006-01-02 15:04:05")
		}

		items = append(items, purchaseItem{
			AppID:                app.ID,
			BundleID:             app.BundleID,
			Name:                 app.Name,
			Version:              app.Version,
			TVOSVersion:          tvosVerByID[app.ID],
			PurchaseDate:         purchaseDate,
			Platforms:            app.Platforms,
			HiddenFromStorefront: hiddenIDs[app.ID],
		})
	}
	return items
}

func parsePurchaseItemsToApps(items []purchaseItem) ([]appstore.App, map[int64]bool, map[int64]string) {
	apps := make([]appstore.App, 0, len(items))
	hiddenIDs := make(map[int64]bool)
	tvosVerByID := make(map[int64]string)
	for _, item := range items {
		apps = append(apps, appstore.App{
			ID:           item.AppID,
			BundleID:     item.BundleID,
			Name:         item.Name,
			Version:      item.Version,
			PurchaseDate: parseCommerceDate(item.PurchaseDate),
			Platforms:    item.Platforms,
		})
		if item.HiddenFromStorefront && item.AppID > 0 {
			hiddenIDs[item.AppID] = true
		}
		if item.TVOSVersion != "" && item.AppID > 0 {
			tvosVerByID[item.AppID] = item.TVOSVersion
		}
	}
	return apps, hiddenIDs, tvosVerByID
}

func readPurchasesDiskCache(acc appstore.Account) ([]appstore.App, map[int64]bool, map[int64]string, time.Time, bool) {
	path := purchasesCacheFilePath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, nil, nil, time.Time{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return nil, nil, nil, time.Time{}, false
	}

	var payload purchasesDiskCache
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, nil, nil, time.Time{}, false
	}

	if payload.DSID != "" && acc.DirectoryServicesID != "" && payload.DSID != acc.DirectoryServicesID {
		return nil, nil, nil, time.Time{}, false
	}

	if payload.Items == nil {
		return nil, nil, nil, time.Time{}, false
	}

	fetchedAt := info.ModTime()
	if payload.FetchedAt > 0 {
		fetchedAt = time.Unix(payload.FetchedAt, 0)
	} else if payload.UpdatedAt != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, payload.UpdatedAt); parseErr == nil {
			fetchedAt = parsed
		}
	}

	apps, hiddenIDs, tvosVerByID := parsePurchaseItemsToApps(payload.Items)
	return apps, hiddenIDs, tvosVerByID, fetchedAt, true
}

func writePurchasesDiskCache(acc appstore.Account, apps []appstore.App, hiddenIDs map[int64]bool, tvosVerByID map[int64]string, fetchedAt time.Time) {
	if fetchedAt.IsZero() {
		fetchedAt = time.Now()
	}
	payload := purchasesDiskCache{
		DSID:      acc.DirectoryServicesID,
		Email:     acc.Email,
		FetchedAt: fetchedAt.Unix(),
		UpdatedAt: fetchedAt.UTC().Format(time.RFC3339),
		Items:     buildPurchaseItems(apps, hiddenIDs, tvosVerByID),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(purchasesCacheFilePath(), data, 0o644)
}

// loadPurchases returns the cached purchase list for acc, loading it from
// purchases_cache.json in the current folder when available, or fetching from
// Apple when the file does not exist, belongs to another account, or force is set.
func loadPurchases(acc appstore.Account, force bool) ([]appstore.App, map[int64]bool, map[int64]string, time.Time, bool, error) {
	purchasesState.Lock()

	if !force && purchasesState.dsid == acc.DirectoryServicesID && purchasesState.apps != nil {
		apps, hiddenIDs, tvosVerByID, at := purchasesState.apps, purchasesState.hiddenIDs, purchasesState.tvosVerByID, purchasesState.fetchedAt
		purchasesState.Unlock()
		return apps, hiddenIDs, tvosVerByID, at, true, nil
	}

	if !force {
		if diskApps, diskHidden, diskTVOS, diskAt, ok := readPurchasesDiskCache(acc); ok {
			purchasesState.dsid = acc.DirectoryServicesID
			purchasesState.apps = diskApps
			purchasesState.hiddenIDs = diskHidden
			purchasesState.tvosVerByID = diskTVOS
			purchasesState.fetchedAt = diskAt
			purchasesState.Unlock()
			return diskApps, diskHidden, diskTVOS, diskAt, true, nil
		}
	}

	if load := purchasesState.inflight; load != nil {
		purchasesState.Unlock()
		<-load.done
		return load.apps, load.hiddenIDs, load.tvosVerByID, time.Now(), false, load.err
	}

	load := &purchasesLoad{done: make(chan struct{})}
	purchasesState.inflight = load
	purchasesState.Unlock()

	_, apps, hiddenIDs, tvosVerByID, err := ownedAppsWithRetry(acc)

	purchasesState.Lock()
	purchasesState.inflight = nil
	now := time.Now()
	if err == nil {
		purchasesState.dsid = acc.DirectoryServicesID
		purchasesState.apps = apps
		purchasesState.hiddenIDs = hiddenIDs
		purchasesState.tvosVerByID = tvosVerByID
		purchasesState.fetchedAt = now
		writePurchasesDiskCache(acc, apps, hiddenIDs, tvosVerByID, now)
	}
	purchasesState.Unlock()

	load.apps, load.hiddenIDs, load.tvosVerByID, load.err = apps, hiddenIDs, tvosVerByID, err
	close(load.done)

	return apps, hiddenIDs, tvosVerByID, now, false, err
}

// invalidatePurchasesCache drops the in-memory cached list, e.g. after logout.
func invalidatePurchasesCache() {
	purchasesState.Lock()
	purchasesState.dsid = ""
	purchasesState.apps = nil
	purchasesState.hiddenIDs = nil
	purchasesState.tvosVerByID = nil
	purchasesState.fetchedAt = time.Time{}
	purchasesState.Unlock()
}

func handleAPIPurchases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	info, err := dependencies.AppStore.AccountInfo()
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "you must log in to view purchases")
		return
	}

	force := r.URL.Query().Get("refresh") == "1"

	apps, hiddenIDs, tvosVerByID, fetchedAt, cached, err := loadPurchases(info.Account, force)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := buildPurchaseItems(apps, hiddenIDs, tvosVerByID)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"items":     items,
		"count":     len(items),
		"cached":    cached,
		"fetchedAt": fetchedAt.Unix(),
	})
}
