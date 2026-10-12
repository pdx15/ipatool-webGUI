package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
