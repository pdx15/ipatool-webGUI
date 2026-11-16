package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

// resetVersionMetadataCacheForTest isolates a test from the process-wide cache
// and restores it afterwards.
func resetVersionMetadataCacheForTest(t *testing.T) {
	t.Helper()

	versionMetaCache.Lock()
	previous := versionMetaCache.m
	versionMetaCache.m = make(map[string]map[string]versionMetaEntry)
	versionMetaCache.Unlock()

	versionMetaDiskOnce = sync.Once{}

	t.Cleanup(func() {
		versionMetaCache.Lock()
		versionMetaCache.m = previous
		versionMetaCache.Unlock()

		versionMetaDiskOnce = sync.Once{}
	})
}

// TestVersionMetadataCacheIsSharedAcrossAccountsAndRuns covers the point of the
// persistent cache: one file for every Apple ID and every GUI run.
func TestVersionMetadataCacheIsSharedAcrossAccountsAndRuns(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), versionMetadataCacheFileName)
	t.Setenv("IPATOOL_GUI_METADATA_CACHE", cachePath)
	resetVersionMetadataCacheForTest(t)

	const (
		appID     = int64(123456789)
		versionID = "987654321"
	)
	cacheKey := versionMetaCacheKey(appID, appstore.PlatformMacOS)

	rememberVersionMetadata(appID, appstore.PlatformMacOS, versionID, versionMetaEntry{
		DisplayVersion:   "2.8.0",
		ReleaseDate:      "2026-01-05",
		MinimumOSVersion: "12.0",
	})

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("expected the shared cache file at %q: %v", cachePath, err)
	}

	var payload versionMetadataDiskFile

	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("cache file is not valid JSON: %v", err)
	}

	if payload.Version != versionMetadataCacheVersion {
		t.Fatalf("cache schema version = %d, want %d", payload.Version, versionMetadataCacheVersion)
	}

	entry, ok := payload.Apps[cacheKey][versionID]
	if !ok {
		t.Fatalf("cache file has no %q/%q entry: %s", cacheKey, versionID, data)
	}

	if entry.DisplayVersion != "2.8.0" || entry.ReleaseDate != "2026-01-05" || entry.MinimumOSVersion != "12.0" {
		t.Fatalf("unexpected cached entry: %+v", entry)
	}

	// Nothing account specific may leak into a file shared by all accounts.
	for _, forbidden := range []string{"dsid", "DSID", "email", "Email", "passwordToken"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("shared cache file contains account data %q: %s", forbidden, data)
		}
	}

	// A second account in a later run reuses the same entry without a lookup.
	versionMetaCache.Lock()
	versionMetaCache.m = make(map[string]map[string]versionMetaEntry)
	versionMetaCache.Unlock()
	versionMetaDiskOnce = sync.Once{}

	loadVersionMetadataDiskCache()

	cached, ok := versionMetaCache.m[cacheKey][versionID]
	if !ok {
		t.Fatalf("expected %q/%q after loading the shared cache", cacheKey, versionID)
	}

	if cached != (versionMetaEntry{DisplayVersion: "2.8.0", ReleaseDate: "2026-01-05", MinimumOSVersion: "12.0"}) {
		t.Fatalf("unexpected entry after reload: %+v", cached)
	}
}

// TestVersionMetadataCacheIgnoresCorruptFile keeps a damaged cache from
// breaking the version history.
func TestVersionMetadataCacheIgnoresCorruptFile(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), versionMetadataCacheFileName)
	t.Setenv("IPATOOL_GUI_METADATA_CACHE", cachePath)
	resetVersionMetadataCacheForTest(t)

	if err := os.WriteFile(cachePath, []byte("{\"version\":1,\"apps\":{"), 0o600); err != nil {
		t.Fatalf("failed to write corrupt cache: %v", err)
	}

	loadVersionMetadataDiskCache()

	if len(versionMetaCache.m) != 0 {
		t.Fatalf("corrupt cache produced %d entries, want 0", len(versionMetaCache.m))
	}

	// The next successful lookup rewrites the file.
	rememberVersionMetadata(42, appstore.PlatformMacOS, "7", versionMetaEntry{DisplayVersion: "1.0"})

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("expected a rewritten cache: %v", err)
	}

	var payload versionMetadataDiskFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("rewritten cache is not valid JSON: %v", err)
	}

	if _, ok := payload.Apps[versionMetaCacheKey(42, appstore.PlatformMacOS)]["7"]; !ok {
		t.Fatalf("rewritten cache lost the new entry: %s", data)
	}
}

// TestVersionMetadataCacheIgnoresUnknownSchema lets a future format rewrite the
// file instead of misreading it.
func TestVersionMetadataCacheIgnoresUnknownSchema(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), versionMetadataCacheFileName)
	t.Setenv("IPATOOL_GUI_METADATA_CACHE", cachePath)
	resetVersionMetadataCacheForTest(t)

	payload := `{"version":99,"apps":{"42":{"7":{"displayVersion":"1.0"}}}}`
	if err := os.WriteFile(cachePath, []byte(payload), 0o600); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	loadVersionMetadataDiskCache()

	if len(versionMetaCache.m) != 0 {
		t.Fatalf("unknown schema produced %d entries, want 0", len(versionMetaCache.m))
	}
}

// TestVersionMetadataCacheIgnoresOversizedFile stops a runaway file from being
// parsed into memory.
func TestVersionMetadataCacheIgnoresOversizedFile(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), versionMetadataCacheFileName)
	t.Setenv("IPATOOL_GUI_METADATA_CACHE", cachePath)
	resetVersionMetadataCacheForTest(t)

	if err := os.WriteFile(cachePath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	if err := os.Truncate(cachePath, maxVersionMetadataCacheBytes+1); err != nil {
		t.Fatalf("failed to grow cache: %v", err)
	}

	loadVersionMetadataDiskCache()

	if len(versionMetaCache.m) != 0 {
		t.Fatalf("oversized cache produced %d entries, want 0", len(versionMetaCache.m))
	}
}

// TestPersistVersionMetadataCacheLeavesNoTempFiles covers the atomic write path.
func TestPersistVersionMetadataCacheLeavesNoTempFiles(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("IPATOOL_GUI_METADATA_CACHE", filepath.Join(directory, versionMetadataCacheFileName))
	resetVersionMetadataCacheForTest(t)

	rememberVersionMetadata(7, appstore.PlatformMacOS, "1", versionMetaEntry{DisplayVersion: "1.0"})
	rememberVersionMetadata(7, appstore.PlatformMacOS, "1", versionMetaEntry{DisplayVersion: "1.0"})
	rememberVersionMetadata(7, appstore.PlatformMacOS, "2", versionMetaEntry{DisplayVersion: "1.1"})

	matches, err := filepath.Glob(filepath.Join(directory, ".version-metadata-*"))
	if err != nil {
		t.Fatalf("failed to list the cache directory: %v", err)
	}

	if len(matches) != 0 {
		t.Fatalf("temporary cache files left behind: %v", matches)
	}

	info, err := os.Stat(filepath.Join(directory, versionMetadataCacheFileName))
	if err != nil {
		t.Fatalf("expected the cache file: %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("cache file mode = %v, want 0600", mode)
	}
}

// TestRememberVersionMetadataSkipsIncompleteEntries keeps placeholder failures
// out of the shared cache.
func TestRememberVersionMetadataSkipsIncompleteEntries(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), versionMetadataCacheFileName)
	t.Setenv("IPATOOL_GUI_METADATA_CACHE", cachePath)
	resetVersionMetadataCacheForTest(t)

	rememberVersionMetadata(0, appstore.PlatformMacOS, "1", versionMetaEntry{DisplayVersion: "1.0"})
	rememberVersionMetadata(7, appstore.PlatformMacOS, "", versionMetaEntry{DisplayVersion: "1.0"})
	rememberVersionMetadata(7, appstore.PlatformMacOS, "1", versionMetaEntry{})

	if len(versionMetaCache.m) != 0 {
		t.Fatalf("incomplete entries were cached: %+v", versionMetaCache.m)
	}

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("no cache file should be written for incomplete entries, stat error = %v", err)
	}
}
