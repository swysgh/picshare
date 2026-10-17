package auth

import (
	"net/http"
	"net/url"
	"strings"
)

// CSRFCheck enforces stateless origin validation on non-GET admin requests.
// It MUST stay stateless — deriving everything from the request itself —
// because the deployment domain changes over time and no origin allowlist
// may ever be configured (a whitelist would lock the owner out after a
// domain move). Rules:
//
//  1. Sec-Fetch-Site, when present, must be "same-origin" or "none".
//  2. Otherwise Origin, when present, must have a host:port matching
//     r.Host (scheme-insensitive to tolerate http/https mixes; r.Host is
//     trustworthy behind nginx's proxy_set_header Host).
//  3. Neither header present → allow (curl, go test, smoke.sh and other
//     non-browser clients have no CSRF surface).
func CSRFCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" {
		return s == "same-origin" || s == "same-site" || s == "none"
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
