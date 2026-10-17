package auth

import (
	"crypto/subtle"
	"net/http"
)

func BasicAuth(user, pass string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="picshare admin", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// Both comparisons always run; combining with || would let the
		// first mismatch short-circuit and leak timing signal.
		uOk := subtle.ConstantTimeCompare([]byte(u), []byte(user))
		pOk := subtle.ConstantTimeCompare([]byte(p), []byte(pass))
		if uOk&pOk != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="picshare admin", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
