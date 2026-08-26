package httpapi

import "net/http"

type secureHandler struct{ next http.Handler }

func withSecurityHeaders(next http.Handler) http.Handler { return secureHandler{next: next} }

func (handler secureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'")
	handler.next.ServeHTTP(w, r)
}
