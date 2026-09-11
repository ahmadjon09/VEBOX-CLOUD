package main

import (
	"net/http"
	"strings"
)

func (app *App) hotlinkAllowed(refHost, reqHost string) bool {
	if refHost == "" || refHost == strings.ToLower(reqHost) {
		return true
	}
	for _, h := range app.cfg.HotlinkHosts {
		if h == refHost {
			return true
		}
	}
	return false
}

func (app *App) hotlinkGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.cfg.HotlinkProtect || app.auth.SessionUser(r) != nil || UserFrom(r.Context()) != nil {
			next.ServeHTTP(w, r)
			return
		}
		ref := r.Header.Get("Referer")
		if ref == "" {
			ref = r.Header.Get("Origin")
		}
		if app.hotlinkAllowed(urlHost(ref), r.Host) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			writeErr(w, http.StatusForbidden, "hotlink_denied",
				"Bu fayl tashqi saytdan ochilishi taqiqlangan")
			return
		}
		writeMissingImageStatus(w, http.StatusForbidden)
	})
}
