package cmd

import (
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

const (
	// localesRoute is the URL prefix under which the localization files are served.
	localesRoute = "/assets/locales/"
	// localesDirName is the directory inside the embedded assets that holds one
	// JSON file per language plus index.json describing the available languages.
	localesDirName = "locales"
	// localeIndexFileName is the manifest listing available languages and the default one.
	localeIndexFileName = "index.json"
	// localeFileNamePattern only accepts flat, plain JSON names such as ru.json or
	// pt-BR.json, so the handler can never escape the locales directory or serve
	// other embedded assets.
	localeFileNamePattern = `^[a-z]{2,8}(-[a-z0-9]{2,8})?\.json$`
)

var localeFileNameRegexp = regexp.MustCompile(localeFileNamePattern)

// handleLocaleAsset serves a single localization file, e.g. /assets/locales/ru.json.
// Every language lives in its own file, which lets translators work on one file
// at a time and lets the client download only the language it actually shows.
func handleLocaleAsset(assets fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, localesRoute)
		if !localeFileNameRegexp.MatchString(name) {
			http.NotFound(w, r)
			return
		}

		data, err := fs.ReadFile(assets, path.Join(localesDirName, name))
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		w.WriteHeader(http.StatusOK)

		if r.Method == http.MethodHead {
			return
		}

		_, _ = w.Write(data)
	}
}
