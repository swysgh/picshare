package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func csrfHandler() http.Handler {
	return CSRFCheck(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func doCSRF(method string, headers map[string]string, host string) int {
	req := httptest.NewRequest(method, "/api/admin/mkdir", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if host != "" {
		req.Host = host
	}
	w := httptest.NewRecorder()
	csrfHandler().ServeHTTP(w, req)
	return w.Code
}

func TestCSRF(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		headers map[string]string
		host    string
		want    int
	}{
		{"no headers POST passes (curl/tests)", http.MethodPost, nil, "", 200},
		{"Sec-Fetch-Site same-origin passes", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin"}, "", 200},
		{"Sec-Fetch-Site none passes", http.MethodPost, map[string]string{"Sec-Fetch-Site": "none"}, "", 200},
		{"Sec-Fetch-Site cross-site rejected", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site"}, "", 403},
		{"Origin mismatched host rejected", http.MethodPost, map[string]string{"Origin": "https://evil.com"}, "gallery.example.com", 403},
		{"Origin matching host passes", http.MethodPost, map[string]string{"Origin": "https://gallery.example.com"}, "gallery.example.com", 200},
		{"Origin scheme-mismatched but host match passes", http.MethodPost, map[string]string{"Origin": "http://gallery.example.com"}, "gallery.example.com", 200},
		{"Origin with port matching Host passes", http.MethodPost, map[string]string{"Origin": "http://127.0.0.1:8080"}, "127.0.0.1:8080", 200},
		{"Origin with port mismatching Host rejected", http.MethodPost, map[string]string{"Origin": "http://127.0.0.1:9999"}, "127.0.0.1:8080", 403},
		{"GET with cross-site Sec-Fetch passes", http.MethodGet, map[string]string{"Sec-Fetch-Site": "cross-site"}, "", 200},
		{"GET with evil Origin passes", http.MethodGet, map[string]string{"Origin": "https://evil.com"}, "gallery.example.com", 200},
		{"Sec-Fetch-Site takes precedence over bad Origin", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "https://evil.com"}, "gallery.example.com", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := doCSRF(tc.method, tc.headers, tc.host); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
