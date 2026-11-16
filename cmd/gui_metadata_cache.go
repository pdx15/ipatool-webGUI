package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

const (
	versionMetadataCacheFileName = "version_metadata_cache.json"
	versionMetadataCacheVersion  = 1
	// A file this large is either foreign or corrupted; ignore it and rewrite.
	maxVersionMetadataCacheBytes = 8 << 20
)

// versionMetadataDiskFile is the on-disk form of the build metadata cache.
//
// Display versions, release dates and minimum OS versions are properties of a
// build ID, not of an account, so one file serves every Apple ID signed in on
// this machine. Keys match versionMetaCacheKey: "appID" for the default and
// iPhone platform, "<platform>:appID" otherwise.
type versionMetadataDiskFile struct {
	Version   int                                        `json:"version"`
	UpdatedAt string                                     `json:"updatedAt,omitempty"`
	Apps      map[string]map[string]versionMetaDiskEntry `json:"apps"`
}

type versionMetaDiskEntry struct {
	DisplayVersion   string `json:"displayVersion"`
	ReleaseDate      string `json:"releaseDate"`
	MinimumOSVersion string `json:"minimumOSVersion,omitempty"`
}

var (
	versionMetaDiskOnce sync.Once
	versionMetaDiskMu   sync.Mutex
	// versionMetaCachePathOverride lets tests point the cache at a temp file.
	versionMetaCachePathOverride string
)

// versionMetadataCachePath resolves the shared cache file. IPATOOL_GUI_METADATA_CACHE
// overrides the location, which also keeps tests away from the real state
// directory.
func versionMetadataCachePath() string {
	if override := strings.TrimSpace(os.Getenv("IPATOOL_GUI_METADATA_CACHE")); override != "" {
		return override
	}
	if override := strings.TrimSpace(versionMetaCachePathOverride); override != "" {
		return override
	}

	return filepath.Join(guiPurchasesCacheDirectory(), versionMetadataCacheFileName)
}

// loadVersionMetadataDiskCache merges the shared cache into the in-memory cache
// once per process, so the version history is filled after a restart without
// asking Apple again.
func loadVersionMetadataDiskCache() {
	versionMetaDiskOnce.Do(func() {
		path := versionMetadataCachePath()

		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() > maxVersionMetadataCacheBytes {
			return
		}

		data, err := os.ReadFile(path)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			return
		}

		var payload versionMetadataDiskFile
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}

		if payload.Version != versionMetadataCacheVersion || payload.Apps == nil {
			return
		}

		versionMetaCache.Lock()
		defer versionMetaCache.Unlock()

		for appKey, versions := range payload.Apps {
			if appKey == "" || len(versions) == 0 {
				continue
			}

			if versionMetaCache.m[appKey] == nil {
				versionMetaCache.m[appKey] = make(map[string]versionMetaEntry, len(versions))
			}

			for versionID, entry := range versions {
				if versionID == "" {
					continue
				}

				// Entries fetched in this process win: they are never older.
				if _, exists := versionMetaCache.m[appKey][versionID]; exists {
					continue
				}

				versionMetaCache.m[appKey][versionID] = versionMetaEntry{
					DisplayVersion:   entry.DisplayVersion,
					ReleaseDate:      entry.ReleaseDate,
					MinimumOSVersion: entry.MinimumOSVersion,
				}
			}
		}
	})
}

// rememberVersionMetadata stores one build in the shared cache and persists the
// result so other accounts and later runs reuse it.
func rememberVersionMetadata(appID int64, platform appstore.Platform, versionID string, entry versionMetaEntry) {
	if appID == 0 || versionID == "" || entry.DisplayVersion == "" {
		return
	}

	cacheKey := versionMetaCacheKey(appID, platform)

	versionMetaCache.Lock()
	if versionMetaCache.m[cacheKey] == nil {
		versionMetaCache.m[cacheKey] = make(map[string]versionMetaEntry, 1)
	}

	previous, existed := versionMetaCache.m[cacheKey][versionID]
	versionMetaCache.m[cacheKey][versionID] = entry
	versionMetaCache.Unlock()

	if existed && previous == entry {
		return
	}

	persistVersionMetadataCache()
}

// persistVersionMetadataCache writes the whole shared cache through a temporary
// file so a crashed or concurrent write cannot truncate it.
func persistVersionMetadataCache() {
	versionMetaDiskMu.Lock()
	defer versionMetaDiskMu.Unlock()

	payload := versionMetadataDiskFile{
		Version:   versionMetadataCacheVersion,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Apps:      map[string]map[string]versionMetaDiskEntry{},
	}

	versionMetaCache.RLock()
	for appKey, versions := range versionMetaCache.m {
		if appKey == "" || len(versions) == 0 {
			continue
		}

		entries := make(map[string]versionMetaDiskEntry, len(versions))
		for versionID, entry := range versions {
			if versionID == "" || entry.DisplayVersion == "" {
				continue
			}

			entries[versionID] = versionMetaDiskEntry{
				DisplayVersion:   entry.DisplayVersion,
				ReleaseDate:      entry.ReleaseDate,
				MinimumOSVersion: entry.MinimumOSVersion,
			}
		}

		if len(entries) > 0 {
			payload.Apps[appKey] = entries
		}
	}
	versionMetaCache.RUnlock()

	if len(payload.Apps) == 0 {
		return
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}

	path := versionMetadataCachePath()
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return
	}

	temp, err := os.CreateTemp(directory, ".version-metadata-*")
	if err != nil {
		return
	}

	tempPath := temp.Name()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)

		return
	}

	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)

		return
	}

	if err := os.Chmod(tempPath, 0o600); err != nil {
		_ = os.Remove(tempPath)

		return
	}

	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
	}
}
