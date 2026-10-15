package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/majd/ipatool/v2/resources"
)

// The removed-apps catalog is compiled into the binary so single-file builds
// (the macOS releases) ship the search. A build that embeds nothing, or a
// catalog the parser cannot read, would silently disable the feature.
func TestBundledAppsIDListIsUsable(t *testing.T) {
	entries := parseAppsIDList(resources.AppsIDList)
	if len(entries) == 0 {
		t.Fatal("the bundled Apps_ID_List.txt produced no entries")
	}

	for _, entry := range entries {
		if entry.AppID <= 0 || entry.Name == "" {
			t.Fatalf("unexpected catalog entry: %+v", entry)
		}
	}

	// A known app from the catalog must be findable through the same matcher
	// the removed-apps search uses.
	found := 0

	for _, entry := range entries {
		if removedMatchRank(entry, "2гис", 0, false) >= 0 {
			found++
		}
	}

	if found == 0 {
		t.Fatal("the bundled catalog did not match a known app name")
	}
}

// fakeAppStore implements the appstore.AppStore methods the GUI helpers use.
// Every other method is a stub: a test that needs one can assign the matching
// function field.
type fakeAppStore struct {
	login              func(appstore.LoginInput) (appstore.LoginOutput, error)
	lookup             func(appstore.LookupInput) (appstore.LookupOutput, error)
	purchase           func(appstore.PurchaseInput) error
	listVersions       func(appstore.ListVersionsInput) (appstore.ListVersionsOutput, error)
	download           func(appstore.DownloadInput) (appstore.DownloadOutput, error)
	getVersionMetadata func(appstore.GetVersionMetadataInput) (appstore.GetVersionMetadataOutput, error)
}

func (f *fakeAppStore) Login(input appstore.LoginInput) (appstore.LoginOutput, error) {
	if f.login == nil {
		return appstore.LoginOutput{}, errors.New("login is not stubbed")
	}

	return f.login(input)
}

func (f *fakeAppStore) AccountInfo() (appstore.AccountInfoOutput, error) {
	return appstore.AccountInfoOutput{}, nil
}

func (f *fakeAppStore) Revoke() error { return nil }

func (f *fakeAppStore) Lookup(input appstore.LookupInput) (appstore.LookupOutput, error) {
	if f.lookup == nil {
		return appstore.LookupOutput{}, errors.New("lookup is not stubbed")
	}

	return f.lookup(input)
}

func (f *fakeAppStore) Search(appstore.SearchInput) (appstore.SearchOutput, error) {
	return appstore.SearchOutput{}, errors.New("search is not stubbed")
}

func (f *fakeAppStore) OwnedApps(appstore.OwnedAppsInput) (appstore.OwnedAppsOutput, error) {
	return appstore.OwnedAppsOutput{}, errors.New("ownedApps is not stubbed")
}

func (f *fakeAppStore) Purchase(input appstore.PurchaseInput) error {
	if f.purchase == nil {
		return nil
	}

	return f.purchase(input)
}

func (f *fakeAppStore) Download(input appstore.DownloadInput) (appstore.DownloadOutput, error) {
	if f.download == nil {
		return appstore.DownloadOutput{}, errors.New("download is not stubbed")
	}

	return f.download(input)
}

func (f *fakeAppStore) ReplicateSinf(appstore.ReplicateSinfInput) error { return nil }

func (f *fakeAppStore) ListVersions(input appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
	if f.listVersions == nil {
		return appstore.ListVersionsOutput{}, errors.New("listVersions is not stubbed")
	}

	return f.listVersions(input)
}

func (f *fakeAppStore) GetVersionMetadata(input appstore.GetVersionMetadataInput) (appstore.GetVersionMetadataOutput, error) {
	if f.getVersionMetadata == nil {
		return appstore.GetVersionMetadataOutput{}, errors.New("getVersionMetadata is not stubbed")
	}

	return f.getVersionMetadata(input)
}

func (f *fakeAppStore) Bag(appstore.BagInput) (appstore.BagOutput, error) {
	return appstore.BagOutput{}, errors.New("bag is not stubbed")
}

// withFakeAppStore installs a fake backend for one test and restores the global
// dependencies afterwards.
func withFakeAppStore(t *testing.T, fake *fakeAppStore) {
	t.Helper()

	previous := dependencies
	dependencies = Dependencies{AppStore: fake}

	t.Cleanup(func() { dependencies = previous })
}

// The batch license probe must use ipatool's ListVersions (the same
// direct-download request the downloader sends) and report a missing license
// without retrying it as an expired session.
func TestListVersionsWithRetryReportsMissingLicense(t *testing.T) {
	calls := 0
	withFakeAppStore(t, &fakeAppStore{
		listVersions: func(appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
			calls++

			return appstore.ListVersionsOutput{}, appstore.ErrLicenseRequired
		},
	})

	_, _, err := listVersionsWithRetry(appstore.Account{Email: "user@example.com"}, appstore.App{ID: 12345}, appstore.PlatformIPhone)
	if !errors.Is(err, appstore.ErrLicenseRequired) {
		t.Fatalf("expected ErrLicenseRequired, got %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected a single probe, got %d", calls)
	}
}

// An expired password token is refreshed once, then the probe is repeated with
// the refreshed account.
func TestListVersionsWithRetryRefreshesExpiredToken(t *testing.T) {
	probes := 0
	logins := 0

	withFakeAppStore(t, &fakeAppStore{
		listVersions: func(input appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
			probes++

			if probes == 1 {
				if input.Account.Password != "old-password" {
					t.Errorf("first probe used password %q", input.Account.Password)
				}

				return appstore.ListVersionsOutput{}, appstore.ErrPasswordTokenExpired
			}

			if input.Account.Password != "new-password" {
				t.Errorf("second probe used password %q", input.Account.Password)
			}

			return appstore.ListVersionsOutput{
				ExternalVersionIdentifiers: []string{"111", "222"},
				LatestExternalVersionID:    "222",
			}, nil
		},
		login: func(input appstore.LoginInput) (appstore.LoginOutput, error) {
			logins++

			return appstore.LoginOutput{Account: appstore.Account{Email: input.Email, Password: "new-password"}}, nil
		},
	})

	account, output, err := listVersionsWithRetry(
		appstore.Account{Email: "user@example.com", Password: "old-password"},
		appstore.App{ID: 12345},
		appstore.PlatformIPhone,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if logins != 1 || probes != 2 {
		t.Fatalf("expected one login and two probes, got %d and %d", logins, probes)
	}

	if output.LatestExternalVersionID != "222" || len(output.ExternalVersionIdentifiers) != 2 {
		t.Fatalf("unexpected output: %+v", output)
	}

	if account.Password != "new-password" {
		t.Fatalf("expected the refreshed account back, got %q", account.Password)
	}
}

// The version string shown next to a batch result comes from the catalog
// lookup and stays empty when the app has no catalog entry.
func TestCatalogVersionFallsBackToEmpty(t *testing.T) {
	withFakeAppStore(t, &fakeAppStore{
		lookup: func(input appstore.LookupInput) (appstore.LookupOutput, error) {
			if input.AppID != 12345 {
				t.Errorf("lookup used app ID %d", input.AppID)
			}

			return appstore.LookupOutput{App: appstore.App{ID: input.AppID, Version: "1.2.3"}}, nil
		},
	})

	if got := catalogVersion(appstore.Account{}, appstore.App{ID: 12345}, appstore.PlatformIPhone); got != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %q", got)
	}

	withFakeAppStore(t, &fakeAppStore{
		lookup: func(appstore.LookupInput) (appstore.LookupOutput, error) {
			return appstore.LookupOutput{}, appstore.ErrAppNotFound
		},
	})

	if got := catalogVersion(appstore.Account{}, appstore.App{ID: 12345}, appstore.PlatformIPhone); got != "" {
		t.Fatalf("expected no version for a delisted app, got %q", got)
	}
}

func TestRunBatchCheckItemSkipsPurchaseWhenAlreadyLicensed(t *testing.T) {
	probes := 0
	purchases := 0

	withFakeAppStore(t, &fakeAppStore{
		listVersions: func(appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
			probes++
			return appstore.ListVersionsOutput{
				ExternalVersionIdentifiers: []string{"100", "200"},
				LatestExternalVersionID:    "200",
			}, nil
		},
		purchase: func(appstore.PurchaseInput) error {
			purchases++
			return nil
		},
		lookup: func(appstore.LookupInput) (appstore.LookupOutput, error) {
			return appstore.LookupOutput{App: appstore.App{ID: 12345, Version: "2.0"}}, nil
		},
	})

	res := runBatchCheckItem(
		appstore.App{ID: 12345},
		appstore.Account{Email: "user@example.com"},
		appstore.PlatformIPhone,
		true,
		minBatchTimeout,
	)

	if res.checkErr != nil {
		t.Fatalf("unexpected check error: %v", res.checkErr)
	}
	if probes != 1 {
		t.Fatalf("expected 1 probe, got %d", probes)
	}
	if purchases != 0 {
		t.Fatalf("expected 0 purchases for an already-licensed app, got %d", purchases)
	}
	if res.version != "2.0" || res.out.LatestExternalVersionID != "200" {
		t.Fatalf("unexpected batch check result: %+v", res)
	}
}

func TestRunBatchCheckItemPurchasesOnlyWhenLicenseRequired(t *testing.T) {
	probes := 0
	purchases := 0

	withFakeAppStore(t, &fakeAppStore{
		listVersions: func(appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
			probes++
			if probes == 1 {
				return appstore.ListVersionsOutput{}, appstore.ErrLicenseRequired
			}
			return appstore.ListVersionsOutput{
				ExternalVersionIdentifiers: []string{"300"},
				LatestExternalVersionID:    "300",
			}, nil
		},
		purchase: func(appstore.PurchaseInput) error {
			purchases++
			return nil
		},
		lookup: func(appstore.LookupInput) (appstore.LookupOutput, error) {
			return appstore.LookupOutput{App: appstore.App{ID: 12345, Version: "3.0"}}, nil
		},
	})

	res := runBatchCheckItem(
		appstore.App{ID: 12345},
		appstore.Account{Email: "user@example.com"},
		appstore.PlatformIPhone,
		true,
		minBatchTimeout,
	)

	if res.checkErr != nil {
		t.Fatalf("unexpected check error after purchase: %v", res.checkErr)
	}
	if probes != 2 || purchases != 1 {
		t.Fatalf("expected 2 probes and 1 purchase, got %d probes and %d purchases", probes, purchases)
	}
	if res.version != "3.0" || res.out.LatestExternalVersionID != "300" {
		t.Fatalf("unexpected batch check result: %+v", res)
	}
}

func TestExecuteDownloadJobDownloadsBeforePurchasing(t *testing.T) {
	downloads := 0
	purchases := 0

	withFakeAppStore(t, &fakeAppStore{
		download: func(appstore.DownloadInput) (appstore.DownloadOutput, error) {
			downloads++
			return appstore.DownloadOutput{
				DestinationPath: "/tmp/owned.pkg",
				Sinfs:           []appstore.Sinf{{ID: 1, Data: []byte("sinf")}},
			}, nil
		},
		purchase: func(appstore.PurchaseInput) error {
			purchases++
			return nil
		},
	})

	job := &DownloadJob{ID: "job-owned", AppID: 12345, AppName: "Owned App", Status: "queued"}
	activeJobs.set(job)

	executeDownloadJob(job, downloadRequestPayload{
		AppID:    12345,
		AppName:  "Owned App",
		Purchase: true,
	}, appstore.Account{Email: "user@example.com"})

	if downloads != 1 {
		t.Fatalf("expected 1 download call, got %d", downloads)
	}
	if purchases != 0 {
		t.Fatalf("expected 0 purchase calls when download succeeds directly, got %d", purchases)
	}
	if job.Status != "completed" {
		t.Fatalf("expected completed job, got %s (%s)", job.Status, job.Error)
	}
}

func TestLookupVersionMetadataReturnsAndCachesMinimumOSVersion(t *testing.T) {
	const testAppID = int64(987654321)
	versionID := "test-version-minimum-os"
	appKey := appIDKey(testAppID)

	versionMetaCache.Lock()
	previous, hadPrevious := versionMetaCache.m[appKey]
	delete(versionMetaCache.m, appKey)
	versionMetaCache.Unlock()
	t.Cleanup(func() {
		versionMetaCache.Lock()
		defer versionMetaCache.Unlock()
		if hadPrevious {
			versionMetaCache.m[appKey] = previous
		} else {
			delete(versionMetaCache.m, appKey)
		}
	})

	calls := 0
	withFakeAppStore(t, &fakeAppStore{
		getVersionMetadata: func(input appstore.GetVersionMetadataInput) (appstore.GetVersionMetadataOutput, error) {
			calls++
			if input.App.ID != testAppID || input.VersionID != versionID {
				t.Errorf("metadata lookup used app %d, version %q", input.App.ID, input.VersionID)
			}
			return appstore.GetVersionMetadataOutput{
				DisplayVersion:   "4.2.1",
				MinimumOSVersion: "15.0",
			}, nil
		},
	})

	result, err := lookupVersionMetadata("", testAppID, versionID)
	if err != nil {
		t.Fatalf("metadata lookup failed: %v", err)
	}
	if got := result["minimumOSVersion"]; got != "15.0" {
		t.Fatalf("minimumOSVersion = %v, want 15.0", got)
	}

	cached, err := lookupVersionMetadata("", testAppID, versionID)
	if err != nil {
		t.Fatalf("cached metadata lookup failed: %v", err)
	}
	if got := cached["minimumOSVersion"]; got != "15.0" {
		t.Fatalf("cached minimumOSVersion = %v, want 15.0", got)
	}
	if calls != 1 {
		t.Fatalf("expected one App Store metadata request, got %d", calls)
	}
}

// The job status must mirror the byte counts ipatool writes into the progress
// bar, including the total size reported by the App Store response.
func TestDownloadProgressSamplesBarState(t *testing.T) {
	progress := newDownloadProgress()
	job := &DownloadJob{ID: "job-1"}

	// ipatool calls ChangeMax64 once the response size is known.
	progress.bar.ChangeMax64(1000)

	if _, err := progress.bar.Write(make([]byte, 250)); err != nil {
		t.Fatalf("failed to feed the bar: %v", err)
	}

	progress.sample(job)

	if job.BytesRead != 250 {
		t.Fatalf("expected 250 bytes read, got %d", job.BytesRead)
	}

	if job.TotalBytes != 1000 {
		t.Fatalf("expected a 1000 byte total, got %d", job.TotalBytes)
	}

	if job.Progress != 25 {
		t.Fatalf("expected 25%%, got %v%%", job.Progress)
	}
}

// While the total size is unknown the job reports bytes only, instead of a
// meaningless percentage.
func TestDownloadProgressWithoutKnownTotal(t *testing.T) {
	progress := newDownloadProgress()
	job := &DownloadJob{ID: "job-2"}

	if _, err := progress.bar.Write(make([]byte, 512)); err != nil {
		t.Fatalf("failed to feed the bar: %v", err)
	}

	progress.sample(job)

	if job.BytesRead != 512 {
		t.Fatalf("expected 512 bytes read, got %d", job.BytesRead)
	}

	if job.TotalBytes != 0 {
		t.Fatalf("expected no total, got %d", job.TotalBytes)
	}

	if job.Progress != 0 {
		t.Fatalf("expected no percentage, got %v", job.Progress)
	}
}

// A response without a content length makes ipatool refuse to update the bar.
// The download must be retried without live progress instead of failing.
func TestDownloadWithRetryDropsProgressBarWhenUnsupported(t *testing.T) {
	downloads := 0
	withFakeAppStore(t, &fakeAppStore{
		download: func(input appstore.DownloadInput) (appstore.DownloadOutput, error) {
			downloads++

			if input.Progress != nil {
				return appstore.DownloadOutput{}, errors.New("can not set bar progress: current number exceeds max")
			}

			return appstore.DownloadOutput{DestinationPath: "/tmp/example.ipa"}, nil
		},
	})

	progress := newDownloadProgress()

	_, out, _, err := downloadWithRetry(downloadTaskInput{
		Account:  appstore.Account{Email: "user@example.com"},
		App:      appstore.App{ID: 12345},
		Progress: progress.bar,
	})
	if err != nil {
		t.Fatalf("expected the retry to succeed, got %v", err)
	}

	if downloads != 2 {
		t.Fatalf("expected two attempts, got %d", downloads)
	}

	if out.DestinationPath != "/tmp/example.ipa" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestIsProgressBarSetupError(t *testing.T) {
	if !isProgressBarSetupError(errors.New("can not set bar progress: current number exceeds max")) {
		t.Error("expected the bar setup error to be recognised")
	}

	if isProgressBarSetupError(errors.New("failed to download file: network is unreachable")) {
		t.Error("unrelated errors must not be recognised")
	}

	if isProgressBarSetupError(nil) {
		t.Error("nil must not be recognised")
	}
}

func TestFormatTransferSpeed(t *testing.T) {
	for _, testCase := range []struct {
		bytesPerSecond float64
		expected       string
	}{
		{512, "512 B/s"},
		{2048, "2.0 KB/s"},
		{3 * 1024 * 1024, "3.00 MB/s"},
	} {
		if got := formatTransferSpeed(testCase.bytesPerSecond); got != testCase.expected {
			t.Errorf("formatTransferSpeed(%v) = %q, want %q", testCase.bytesPerSecond, got, testCase.expected)
		}
	}
}

// Display metadata (icon, developer, size, rating) is merged into search
// results by track ID, because ipatool's own App type carries CLI fields only.
func TestAppCardsMergesCatalogDisplayMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("id"); got != "111,222" {
			t.Errorf("lookup requested IDs %q", got)
		}
		if got := r.URL.Query().Get("entity"); got != "software" {
			t.Errorf("lookup requested entity %q, want %q", got, "software")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resultCount":1,"results":[
			{"trackId":111,"artworkUrl60":"https://example.com/60.png","artworkUrl512":"https://example.com/512.png",
			 "artistName":"Example Studio","sellerName":"Example Inc.","formattedPrice":"Free",
			 "fileSizeBytes":"1048576","averageUserRating":4.5,"userRatingCount":42}
		]}`))
	}))
	defer server.Close()

	previousURL := displayLookupURL
	displayLookupURL = server.URL
	t.Cleanup(func() { displayLookupURL = previousURL })

	apps := []appstore.App{
		{ID: 111, BundleID: "com.example.one", Name: "First", Version: "1.0"},
		{ID: 222, BundleID: "com.example.two", Name: "Second", Version: "2.0"},
	}

	cards := appCards(apps)
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}

	if cards[0].Name != "First" || cards[0].BundleID != "com.example.one" {
		t.Fatalf("ipatool fields were lost: %+v", cards[0])
	}

	if cards[0].ArtworkURL512 != "https://example.com/512.png" || cards[0].ArtistName != "Example Studio" {
		t.Fatalf("display metadata was not merged: %+v", cards[0])
	}

	if cards[0].FileSizeBytes != 1048576 {
		t.Fatalf("expected the quoted size to be parsed, got %d", cards[0].FileSizeBytes)
	}

	if cards[0].AverageUserRating != 4.5 || cards[0].UserRatingCount != 42 {
		t.Fatalf("expected the rating to be merged, got %+v", cards[0])
	}

	// The second app is not part of the catalog answer: the card keeps the
	// placeholders the frontend renders.
	if cards[1].ArtworkURL512 != "" || cards[1].FileSizeBytes != 0 {
		t.Fatalf("unexpected metadata on the unmatched card: %+v", cards[1])
	}
}

// A failing catalog lookup must never break the search response.
func TestAppCardsSurvivesCatalogFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	previousURL := displayLookupURL
	displayLookupURL = server.URL
	t.Cleanup(func() { displayLookupURL = previousURL })

	cards := appCards([]appstore.App{{ID: 111, Name: "First"}})
	if len(cards) != 1 || cards[0].Name != "First" {
		t.Fatalf("search results must survive a failed lookup: %+v", cards)
	}

	if cards[0].ArtworkURL512 != "" {
		t.Fatalf("expected no artwork after a failed lookup: %+v", cards[0])
	}
}

func TestExecuteDownloadJobMacOSSkipsReplicateSinfWithoutSinfs(t *testing.T) {
	var purchasedPlatform appstore.Platform
	replicateCalls := 0
	attempts := 0

	withFakeAppStore(t, &fakeAppStore{
		download: func(input appstore.DownloadInput) (appstore.DownloadOutput, error) {
			attempts++
			if input.Platform != appstore.PlatformMacOS {
				t.Errorf("download platform = %q, want %q", input.Platform, appstore.PlatformMacOS)
			}
			if attempts == 1 {
				return appstore.DownloadOutput{}, appstore.ErrLicenseRequired
			}
			return appstore.DownloadOutput{
				DestinationPath: "/tmp/example.pkg",
				Sinfs:           nil,
			}, nil
		},
		purchase: func(input appstore.PurchaseInput) error {
			purchasedPlatform = input.Platform
			return nil
		},
		replicateSinf: func(appstore.ReplicateSinfInput) error {
			replicateCalls++
			return nil
		},
	})

	job := &DownloadJob{ID: "job-macos"}
	executeDownloadJob(job, downloadRequestPayload{
		AppID:    99999,
		AppName:  "Mac App",
		Platform: "macos",
		Purchase: true,
	}, appstore.Account{Email: "user@example.com"})

	if job.Status != "completed" {
		t.Fatalf("expected completed job, got status=%q error=%q", job.Status, job.Error)
	}
	if purchasedPlatform != appstore.PlatformMacOS {
		t.Fatalf("expected purchase platform %q, got %q", appstore.PlatformMacOS, purchasedPlatform)
	}
	if replicateCalls != 0 {
		t.Fatalf("expected 0 replicateSinf calls for macOS .pkg without sinfs, got %d", replicateCalls)
	}
	if job.Warning != "" {
		t.Fatalf("expected no warning for macOS .pkg without sinfs, got %q", job.Warning)
	}
}

func TestParseCommercePurchasesAndMerge(t *testing.T) {
	rawJSON := []byte(`{
		"data": {
			"attributes": {
				"is-complete": true,
				"purchases": [
					{
						"order-id": "M100",
						"invoice-date": "2021-05-10T12:34:56Z",
						"items": [
							{
								"item-id": "568903335",
								"item-name": "1Password 7 - Password Manager",
								"price": "0.00",
								"purchase-date": "2021-05-10T12:34:56Z",
								"kind": "iOS App"
							},
							{
								"item-id": "999999999",
								"item-name": "Some Book",
								"kind": "Book"
							}
						]
					}
				]
			}
		}
	}`)

	seen := make(map[string]struct{})
	apps, isComplete, added, ok := parseCommercePurchasesPage(rawJSON, seen)
	if !ok || !isComplete || added != 1 {
		t.Fatalf("unexpected parse result: ok=%v isComplete=%v added=%d", ok, isComplete, added)
	}
	if len(apps) != 1 || apps[0].ID != 568903335 || apps[0].Name != "1Password 7 - Password Manager" {
		t.Fatalf("unexpected parsed apps: %+v", apps)
	}

	daapApps := []appstore.App{
		{ID: 111, BundleID: "com.example.active", Name: "Active App", Version: "1.0", Platforms: []appstore.Platform{appstore.PlatformIPhone}},
	}
	merged := mergePurchaseHistoryApps(daapApps, apps)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged apps, got %d: %+v", len(merged), merged)
	}

	if _, _, _, errOK := parseCommercePurchasesPage([]byte(`{"errors":[{"status":"401"}]}`), seen); errOK {
		t.Fatal("expected parseCommercePurchasesPage to reject JSON without data.attributes")
	}

	ruDate := parseCommerceDate("10 мая 2021 г.")
	if ruDate.Year() != 2021 || int(ruDate.Month()) != 5 || ruDate.Day() != 10 {
		t.Fatalf("expected 2021-05-10 from Russian date, got %v", ruDate)
	}
}

func TestCompareWebGUIVersions(t *testing.T) {
	if got := normalizeWebGUIVersion("v2.7.0c-webGUI"); got != "2.7.0c" {
		t.Fatalf("expected 2.7.0c, got %q", got)
	}
	if compareWebGUIVersions("2.7.0c-webGUI", "2.7.0-webGUI") <= 0 {
		t.Fatalf("expected 2.7.0c-webGUI > 2.7.0-webGUI")
	}
	if compareWebGUIVersions("2.7.1-webGUI", "2.7.0c-webGUI") <= 0 {
		t.Fatalf("expected 2.7.1-webGUI > 2.7.0c-webGUI")
	}
	if compareWebGUIVersions("2.7.0c-webGUI", "2.7.0c") != 0 {
		t.Fatalf("expected 2.7.0c-webGUI == 2.7.0c")
	}
}

func TestResolveDownloadOutputPathDefaultsToAppsDir(t *testing.T) {
	got := resolveDownloadOutputPath("")
	if filepath.Base(got) != "apps" {
		t.Fatalf("expected default download directory base name 'apps', got %q", got)
	}
	info, err := os.Stat(got)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected default download directory %q to exist: %v", got, err)
	}
	_ = os.Remove(got)

	custom := resolveDownloadOutputPath("  /custom/output/dir  ")
	if custom != "/custom/output/dir" {
		t.Fatalf("expected trimmed custom path, got %q", custom)
	}
}

func TestSanitizeTxtFilename(t *testing.T) {
	if got := sanitizeTxtFilename("purchases-user@example.com.txt"); got != "purchases-user@example.com.txt" {
		t.Fatalf("unexpected sanitized filename: %q", got)
	}
	if got := sanitizeTxtFilename("list:bad*chars"); got != "list_bad_chars.txt" {
		t.Fatalf("expected list_bad_chars.txt, got %q", got)
	}
	if got := sanitizeTxtFilename(""); got != "purchases.txt" {
		t.Fatalf("expected purchases.txt for empty input, got %q", got)
	}
}

func TestPurchasesDiskCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	acc := appstore.Account{
		DirectoryServicesID: "123456789",
		Email:               "user@example.com",
	}
	apps := []appstore.App{
		{
			ID:       1451612172,
			BundleID: "ru.mts.tvh",
			Name:     "KION",
			Version:  "5.20.3",
			Platforms: []appstore.Platform{
				appstore.PlatformIPhone,
				appstore.PlatformIPad,
				appstore.PlatformAppleTV,
			},
		},
	}
	hiddenIDs := map[int64]bool{1451612172: true}
	tvosVerByID := map[int64]string{1451612172: "3.12.1"}
	fetchedAt := time.Unix(1760486400, 0).UTC()

	writePurchasesDiskCache(acc, apps, hiddenIDs, tvosVerByID, fetchedAt)

	gotApps, gotHidden, gotTVOS, gotAt, ok := readPurchasesDiskCache(acc)
	if !ok {
		t.Fatalf("expected readPurchasesDiskCache to succeed")
	}
	if len(gotApps) != 1 || gotApps[0].ID != 1451612172 || gotApps[0].BundleID != "ru.mts.tvh" {
		t.Fatalf("unexpected apps from disk cache: %+v", gotApps)
	}
	if !gotHidden[1451612172] {
		t.Fatalf("expected hidden flag to round-trip")
	}
	if gotTVOS[1451612172] != "3.12.1" {
		t.Fatalf("expected tvOS version 3.12.1, got %q", gotTVOS[1451612172])
	}
	if gotAt.Unix() != fetchedAt.Unix() {
		t.Fatalf("expected fetchedAt %d, got %d", fetchedAt.Unix(), gotAt.Unix())
	}

	otherAcc := appstore.Account{DirectoryServicesID: "999999999"}
	if _, _, _, _, ok := readPurchasesDiskCache(otherAcc); ok {
		t.Fatalf("expected cache for another DSID to be ignored")
	}
}






