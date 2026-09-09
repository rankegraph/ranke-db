// package: rest_http / transport
// type:    adapter
// job:     the two routes outside the API — explorer.html, and the root naming this build
// limits:  static responses, no API surface — neither is part of the OpenAPI contract
package rest_http

import (
	"net/http"

	"github.com/rankegraph/ranke-db/frontend"
	"github.com/rankegraph/ranke-db/internal/version"
)

// explorerHandler serves the embedded explorer.html when enabled allows it, 404
// otherwise — either because config's "explorer" said not to, or because this binary
// was built without the `explorer` tag (frontend.Explorer is then an empty embed.FS).
func explorerHandler(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			http.Error(w, "explorer disabled by config", http.StatusNotFound)
			return
		}
		b, err := frontend.Explorer.ReadFile("dist/explorer.html")
		if err != nil {
			http.Error(w, "explorer not embedded in this build — built without -tags explorer", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	}
}

// rootHandler names the build at "/". The generated router owns "/" as a catch-all, so
// this is registered on "/{$}", which matches the root alone and leaves every unrouted
// path its 404 — a gap in the paths, not an answer the contract owes.
func rootHandler() http.HandlerFunc {
	body := "ranke-db " + version.String() + "\n"
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}
}
