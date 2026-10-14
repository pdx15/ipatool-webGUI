package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

const (
	commercePurchasesPath       = "/commerce/account/purchases"
	commerceConfiguratorUA      = "Configurator/2.17 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6"
	commerceAppStoreLaunchYear  = 2008
	commerceMaxPagesPerYear     = 50
	commerceMaxRedirectHops     = 4
	commerceYearConcurrency     = 4
	commerceCatalogBatchSize    = 100
)

// purchasesCache keeps the last purchase-history result per Apple ID so the
// "Purchase history" tab is populated instantly after the first load. The
// client asks for a fresh copy explicitly with ?refresh=1.
type purchasesCache struct {
	sync.Mutex
	dsid      string
	apps      []appstore.App
	fetchedAt time.Time
	// inflight collapses concurrent loads (e.g. a refresh click while the
	// initial load is still running) into one Apple round-trip.
	inflight *purchasesLoad
}

type purchasesLoad struct {
	done chan struct{}
	apps []appstore.App
	err  error
}

var purchasesState = &purchasesCache{}

// purchaseItem is the JSON shape consumed by the GUI.
type purchaseItem struct {
	AppID        int64               `json:"appId"`
	BundleID     string              `json:"bundleId"`
	Name         string              `json:"name"`
	Version      string              `json:"version,omitempty"`
	PurchaseDate string              `json:"purchaseDate,omitempty"`
	Platforms    []appstore.Platform `json:"platforms,omitempty"`
}

type commercePurchasesResponse struct {
	Data struct {
		Attributes struct {
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
	TrackID  int64  `json:"trackId"`
	BundleID string `json:"bundleId"`
	Version  string `json:"version"`
	Name     string `json:"trackName"`
}

type catalogEnrichmentResponse struct {
	Results []catalogEnrichmentItem `json:"results"`
}

func fetchDAAPOwnedApps(acc appstore.Account) ([]appstore.App, error) {
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

func fetchAllOwnedApps(acc appstore.Account) ([]appstore.App, error) {
	var (
		daapApps     []appstore.App
		daapErr      error
		commerceApps []appstore.App
		commerceErr  error
		wg           sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		daapApps, daapErr = fetchDAAPOwnedApps(acc)
	}()
	go func() {
		defer wg.Done()
		commerceApps, commerceErr = fetchCommercePurchaseHistory(acc)
	}()
	wg.Wait()

	if daapErr != nil && errors.Is(daapErr, appstore.ErrPasswordTokenExpired) {
		return nil, daapErr
	}

	if daapErr != nil {
		if commerceErr == nil && len(commerceApps) > 0 {
			enrichMissingCatalogFields(commerceApps)
			return sortPurchasesByDate(commerceApps), nil
		}
		if errors.Is(commerceErr, appstore.ErrPasswordTokenExpired) {
			return nil, commerceErr
		}
		return nil, daapErr
	}

	merged := mergePurchaseHistoryApps(daapApps, commerceApps)
	enrichMissingCatalogFields(merged)

	return sortPurchasesByDate(merged), nil
}

func fetchCommercePurchaseHistory(acc appstore.Account) ([]appstore.App, error) {
	if strings.TrimSpace(acc.DirectoryServicesID) == "" {
		return nil, nil
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if dependencies.CookieJar != nil {
		client.Jar = dependencies.CookieJar
	}

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

	currentYear := time.Now().Year()
	if currentYear < commerceAppStoreLaunchYear {
		currentYear = commerceAppStoreLaunchYear
	}

	// Fetch the current year first so the initial 302 pod/cookie handshake
	// populates itspod in CookieJar and discovers any updated pod before
	// launching concurrent requests for earlier years.
	firstYearApps, status, err := fetchCommercePurchasesForRange(client, acc, getPod, setPod, fmt.Sprintf("%d-all", currentYear))
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, appstore.ErrPasswordTokenExpired
	}
	if err != nil {
		return nil, err
	}

	if dependencies.CookieJar != nil {
		_ = dependencies.CookieJar.Save()
	}

	type yearResult struct {
		year int
		apps []appstore.App
	}

	years := make([]int, 0, currentYear-commerceAppStoreLaunchYear)
	for y := currentYear - 1; y >= commerceAppStoreLaunchYear; y-- {
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

			apps, _, rangeErr := fetchCommercePurchasesForRange(client, acc, getPod, setPod, fmt.Sprintf("%d-all", y))
			if rangeErr == nil && len(apps) > 0 {
				results[i] = yearResult{year: y, apps: apps}
			}
		}(idx, year)
	}

	wg.Wait()

	if dependencies.CookieJar != nil {
		_ = dependencies.CookieJar.Save()
	}

	allApps := make([]appstore.App, 0, len(firstYearApps)+64)
	allApps = append(allApps, firstYearApps...)
	for _, res := range results {
		allApps = append(allApps, res.apps...)
	}

	return mergePurchaseHistoryApps(nil, allApps), nil
}

func fetchCommercePurchasesForRange(
	client *http.Client,
	acc appstore.Account,
	getPod func() string,
	setPod func(string),
	rangeParam string,
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
		q := url.Values{}
		q.Set("isDeepLink", "false")
		q.Set("isJsonApiFormat", "true")
		if page > 0 {
			q.Set("page", strconv.Itoa(page))
		}
		if rangeParam != "" {
			q.Set("range", rangeParam)
		}
		return fmt.Sprintf("https://%sbuy.itunes.apple.com%s?%s", prefix, commercePurchasesPath, q.Encode())
	}

	doGET := func(rawURL string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", commerceConfiguratorUA)
		req.Header.Set("Accept-Language", "en-us")
		req.Header.Set("X-Apple-Store-Front", acc.StoreFront)
		req.Header.Set("X-Dsid", acc.DirectoryServicesID)
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

	for hop := 0; (res.StatusCode == http.StatusFound || res.StatusCode == http.StatusMovedPermanently || res.StatusCode == http.StatusTemporaryRedirect) && hop < commerceMaxRedirectHops; hop++ {
		if headerPod := strings.TrimSpace(res.Header.Get("pod")); headerPod != "" {
			setPod(headerPod)
		}
		loc := strings.TrimSpace(res.Header.Get("Location"))
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if loc == "" {
			return nil, res.StatusCode, errors.New("commerce purchase history redirect missing Location")
		}
		if locPod := extractPodFromURL(loc); locPod != "" {
			setPod(locPod)
		}
		res, err = doGET(loc)
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

	isComplete := true
	if payload.Data.Attributes.IsComplete != nil {
		isComplete = *payload.Data.Attributes.IsComplete
	}

	addedInvoices := 0
	var apps []appstore.App

	for _, inv := range payload.Data.Attributes.Purchases {
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
		"book", "audiobook", "song", "music", "album", "movie",
		"ringtone", "tone", "podcast", "tv episode", "tv season",
		"icloud", "subscription", "in-app", "in app", "pass",
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
	case strings.Contains(lower, "universal"):
		return []appstore.Platform{appstore.PlatformIPhone, appstore.PlatformIPad}, true
	case strings.Contains(lower, "ipad"):
		return []appstore.Platform{appstore.PlatformIPad}, true
	default:
		return []appstore.Platform{appstore.PlatformIPhone}, true
	}
}

func parseCommerceDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
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
		"02.01.2006",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
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

func enrichMissingCatalogFields(apps []appstore.App) {
	missingIndices := make(map[int64]int)
	missingIDs := make([]int64, 0)

	for idx := range apps {
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

func lookupCatalogEnrichment(ids []string, country string) ([]catalogEnrichmentItem, error) {
	query := url.Values{}
	query.Set("id", strings.Join(ids, ","))
	query.Set("entity", "software,iPadSoftware,macSoftware")
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
func ownedAppsWithRetry(acc appstore.Account) (appstore.Account, []appstore.App, error) {
	refreshed := false

	for attempt := 0; attempt < 2; attempt++ {
		apps, err := fetchAllOwnedApps(acc)
		if err == nil {
			return acc, apps, nil
		}

		if errors.Is(err, appstore.ErrPasswordTokenExpired) && !refreshed {
			if acc.Password == "" {
				return acc, nil, errors.New("password token is expired and no password is stored; log in again to refresh the session")
			}

			refreshedAcc, refreshErr := refreshAccount(acc)
			if refreshErr != nil {
				return acc, nil, refreshErr
			}

			acc = refreshedAcc
			refreshed = true
			continue
		}

		return acc, nil, err
	}

	return acc, nil, errors.New("failed to load purchase history after re-authentication")
}

// loadPurchases returns the cached purchase list for acc, loading it from
// Apple when the cache is empty, belongs to another account, or force is set.
func loadPurchases(acc appstore.Account, force bool) ([]appstore.App, time.Time, bool, error) {
	purchasesState.Lock()

	if !force && purchasesState.dsid == acc.DirectoryServicesID && purchasesState.apps != nil {
		apps, at := purchasesState.apps, purchasesState.fetchedAt
		purchasesState.Unlock()
		return apps, at, true, nil
	}

	if load := purchasesState.inflight; load != nil {
		purchasesState.Unlock()
		<-load.done
		return load.apps, time.Now(), false, load.err
	}

	load := &purchasesLoad{done: make(chan struct{})}
	purchasesState.inflight = load
	purchasesState.Unlock()

	_, apps, err := ownedAppsWithRetry(acc)

	purchasesState.Lock()
	purchasesState.inflight = nil
	now := time.Now()
	if err == nil {
		purchasesState.dsid = acc.DirectoryServicesID
		purchasesState.apps = apps
		purchasesState.fetchedAt = now
	}
	purchasesState.Unlock()

	load.apps, load.err = apps, err
	close(load.done)

	return apps, now, false, err
}

// invalidatePurchasesCache drops the cached list, e.g. after logout.
func invalidatePurchasesCache() {
	purchasesState.Lock()
	purchasesState.dsid = ""
	purchasesState.apps = nil
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

	apps, fetchedAt, cached, err := loadPurchases(info.Account, force)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]purchaseItem, 0, len(apps))
	for _, app := range apps {
		purchaseDate := ""
		if !app.PurchaseDate.IsZero() {
			purchaseDate = app.PurchaseDate.Format("2006-01-02 15:04:05")
		}

		items = append(items, purchaseItem{
			AppID:        app.ID,
			BundleID:     app.BundleID,
			Name:         app.Name,
			Version:      app.Version,
			PurchaseDate: purchaseDate,
			Platforms:    app.Platforms,
		})
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"items":     items,
		"count":     len(items),
		"cached":    cached,
		"fetchedAt": fetchedAt.Unix(),
	})
}
