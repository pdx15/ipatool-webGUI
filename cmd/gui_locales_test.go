package cmd

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

// localeIndex mirrors locales/index.json: the default language plus every
// language shipped with the build.
type localeIndex struct {
	Default   string   `json:"default"`
	Languages []string `json:"languages"`
}

// localeDocument mirrors locales/<code>.json.
type localeDocument struct {
	Meta struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		HTMLLang   string `json:"htmlLang"`
		IntlLocale string `json:"intlLocale"`
	} `json:"meta"`
	Strings map[string]string `json:"strings"`
}

var (
	htmlI18nKeyRegexp = regexp.MustCompile(`data-i18n(?:-title|-placeholder)?="([a-z0-9_]+)"`)
	jsI18nKeyRegexp   = regexp.MustCompile(`\b(?:t|batchText)\('([a-z0-9_]+)'`)
)

// readLocaleIndex loads and validates locales/index.json inside dir.
func readLocaleIndex(t *testing.T, fsys fs.FS, label, dir string) localeIndex {
	t.Helper()

	data, err := fs.ReadFile(fsys, path.Join(dir, localeIndexFileName))
	if err != nil {
		t.Fatalf("%s: cannot read %s: %v", label, localeIndexFileName, err)
	}

	var index localeIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("%s: cannot parse %s: %v", label, localeIndexFileName, err)
	}
	if len(index.Languages) == 0 {
		t.Fatalf("%s: %s lists no languages", label, localeIndexFileName)
	}

	found := false
	for _, code := range index.Languages {
		if code == index.Default {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s: default language %q is not listed in languages %v", label, index.Default, index.Languages)
	}

	return index
}

// readLocaleDocument loads locales/<code>.json inside dir.
func readLocaleDocument(t *testing.T, fsys fs.FS, label, dir, code string) localeDocument {
	t.Helper()

	data, err := fs.ReadFile(fsys, path.Join(dir, code+".json"))
	if err != nil {
		t.Fatalf("%s: cannot read locale file for %q: %v", label, code, err)
	}

	var doc localeDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: cannot parse locale file for %q: %v", label, code, err)
	}
	if doc.Meta.Code != code {
		t.Fatalf("%s: locale file %q declares meta.code %q", label, code, doc.Meta.Code)
	}
	if strings.TrimSpace(doc.Meta.Name) == "" {
		t.Fatalf("%s: locale %q has no meta.name", label, code)
	}
	if len(doc.Strings) == 0 {
		t.Fatalf("%s: locale %q has no strings", label, code)
	}

	for key, value := range doc.Strings {
		if strings.TrimSpace(value) == "" {
			t.Errorf("%s: locale %q has an empty value for key %q", label, code, key)
		}
	}

	return doc
}

// checkLocaleSet validates the whole locale directory: manifest, every language
// file, and full key parity with the default language.
func checkLocaleSet(t *testing.T, fsys fs.FS, label, dir string) (localeIndex, map[string]localeDocument) {
	t.Helper()

	index := readLocaleIndex(t, fsys, label, dir)

	listed := make(map[string]bool, len(index.Languages))
	documents := make(map[string]localeDocument, len(index.Languages))
	for _, code := range index.Languages {
		if listed[code] {
			t.Fatalf("%s: language %q is listed twice", label, code)
		}
		listed[code] = true
		documents[code] = readLocaleDocument(t, fsys, label, dir, code)
	}

	// Every JSON file in the directory must be referenced by the manifest, so a
	// translation cannot be silently forgotten there.
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		t.Fatalf("%s: cannot list %s: %v", label, dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == localeIndexFileName || !strings.HasSuffix(name, ".json") {
			continue
		}
		if code := strings.TrimSuffix(name, ".json"); !listed[code] {
			t.Errorf("%s: %s is not listed in %s", label, name, localeIndexFileName)
		}
	}

	// Key parity: a missing translation would silently fall back to English, and
	// an unknown key is usually a typo.
	reference := documents[index.Default]
	for _, code := range index.Languages {
		doc := documents[code]
		for key := range reference.Strings {
			if _, ok := doc.Strings[key]; !ok {
				t.Errorf("%s: locale %q is missing key %q", label, code, key)
			}
		}
		for key := range doc.Strings {
			if _, ok := reference.Strings[key]; !ok {
				t.Errorf("%s: locale %q has key %q that %q does not define", label, code, key, index.Default)
			}
		}
	}

	return index, documents
}

// checkUIKeysResolve makes sure every key referenced by the markup and by the
// scripts exists in the locale files.
func checkUIKeysResolve(t *testing.T, fsys fs.FS, label string, documents map[string]localeDocument, index localeIndex, files ...string) {
	t.Helper()

	dict := documents[index.Default].Strings
	for _, file := range files {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			t.Fatalf("%s: cannot read %s: %v", label, file, err)
		}
		content := string(data)

		var keys []string
		if strings.HasSuffix(file, ".html") {
			for _, match := range htmlI18nKeyRegexp.FindAllStringSubmatch(content, -1) {
				keys = append(keys, match[1])
			}
		} else {
			for _, match := range jsI18nKeyRegexp.FindAllStringSubmatch(content, -1) {
				keys = append(keys, match[1])
			}
		}

		seen := make(map[string]bool, len(keys))
		for _, key := range keys {
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, ok := dict[key]; !ok {
				t.Errorf("%s: %s uses key %q that is missing from %q", label, file, key, index.Default)
			}
		}
	}
}

func TestEmbeddedLocalesAreValid(t *testing.T) {
	root, err := fs.Sub(embeddedAssets, "gui_assets")
	if err != nil {
		t.Fatalf("cannot open embedded assets: %v", err)
	}

	index, documents := checkLocaleSet(t, root, "gui_assets", localesDirName)

	// Both the GUI shipped in the binary and the static docs demo must define
	// every key their markup and scripts reference.
	checkUIKeysResolve(t, root, "gui_assets", documents, index, "index.html", "app.js")
}

func TestDocsLocalesAreValid(t *testing.T) {
	const docsDir = "docs/assets/locales"
	if _, err := os.Stat("../docs"); err != nil {
		t.Skip("docs demo is not part of this checkout")
	}

	root := os.DirFS("..")
	index, documents := checkLocaleSet(t, root, "docs", docsDir)

	checkUIKeysResolve(t, root, "docs", documents, index, "docs/index.html", "docs/assets/app.js")
}

func TestHandleLocaleAsset(t *testing.T) {
	root, err := fs.Sub(embeddedAssets, "gui_assets")
	if err != nil {
		t.Fatalf("cannot open embedded assets: %v", err)
	}
	handler := handleLocaleAsset(root)

	cases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "index", method: http.MethodGet, path: "/assets/locales/index.json", wantStatus: http.StatusOK},
		{name: "language", method: http.MethodGet, path: "/assets/locales/en.json", wantStatus: http.StatusOK},
		{name: "head", method: http.MethodHead, path: "/assets/locales/ru.json", wantStatus: http.StatusOK},
		{name: "unknown language", method: http.MethodGet, path: "/assets/locales/zz.json", wantStatus: http.StatusNotFound},
		{name: "path traversal", method: http.MethodGet, path: "/assets/locales/../app.js", wantStatus: http.StatusNotFound},
		{name: "encoded traversal", method: http.MethodGet, path: "/assets/locales/..%2fapp.js", wantStatus: http.StatusNotFound},
		{name: "nested path", method: http.MethodGet, path: "/assets/locales/sub/ru.json", wantStatus: http.StatusNotFound},
		{name: "uppercase name", method: http.MethodGet, path: "/assets/locales/RU.JSON", wantStatus: http.StatusNotFound},
		{name: "post", method: http.MethodPost, path: "/assets/locales/ru.json", wantStatus: http.StatusMethodNotAllowed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("%s %s: expected status %d, got %d", tc.method, tc.path, tc.wantStatus, rec.Code)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("%s: unexpected Content-Type %q", tc.path, ct)
			}
			if tc.method == http.MethodGet && !json.Valid(rec.Body.Bytes()) {
				t.Fatalf("%s: response is not valid JSON", tc.path)
			}
		})
	}
}

func TestHandleLocaleAssetServesManifest(t *testing.T) {
	root, err := fs.Sub(embeddedAssets, "gui_assets")
	if err != nil {
		t.Fatalf("cannot open embedded assets: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/locales/"+localeIndexFileName, nil)
	rec := httptest.NewRecorder()
	handleLocaleAsset(root).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var index localeIndex
	if err := json.Unmarshal(rec.Body.Bytes(), &index); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if len(index.Languages) == 0 {
		t.Fatalf("manifest lists no languages")
	}
}
