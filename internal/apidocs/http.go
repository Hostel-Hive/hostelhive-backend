package apidocs

import (
	"embed"
	"net/http"
)

//go:embed assets/index.html assets/initializer.js assets/roles.css assets/swagger-ui.css assets/swagger-ui-bundle.js assets/LICENSE assets/NOTICE assets/swagger-ui-bundle.js.LICENSE.txt
var assets embed.FS

func registerUI(mux *http.ServeMux) {
	spec := JSON()
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		docHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != "HEAD" {
			_, _ = w.Write(spec)
		}
	})
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusTemporaryRedirect)
	})
	for _, item := range []struct{ path, file, contentType string }{
		{"/docs/{$}", "index.html", "text/html; charset=utf-8"},
		{"/docs/initializer.js", "initializer.js", "text/javascript; charset=utf-8"},
		{"/docs/roles.css", "roles.css", "text/css; charset=utf-8"},
		{"/docs/swagger-ui-bundle.js", "swagger-ui-bundle.js", "text/javascript; charset=utf-8"},
		{"/docs/swagger-ui.css", "swagger-ui.css", "text/css; charset=utf-8"},
		{"/docs/LICENSE", "LICENSE", "text/plain; charset=utf-8"},
		{"/docs/NOTICE", "NOTICE", "text/plain; charset=utf-8"},
		{"/docs/swagger-ui-bundle.js.LICENSE.txt", "swagger-ui-bundle.js.LICENSE.txt", "text/plain; charset=utf-8"},
	} {
		data, err := assets.ReadFile("assets/" + item.file)
		if err != nil {
			panic(err)
		}
		mux.HandleFunc("GET "+item.path, func(w http.ResponseWriter, r *http.Request) {
			docHeaders(w)
			w.Header().Set("Content-Type", item.contentType)
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
			if r.Method != "HEAD" {
				_, _ = w.Write(data)
			}
		})
	}
}
func docHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
