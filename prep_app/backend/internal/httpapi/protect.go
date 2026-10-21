package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"prepapp/internal/auth"
)

var (
	errNeedPassword = errors.New("пароль не задан: откройте тренажёр на компьютере, где запущен сервер, и задайте его")
	errNeedLogin    = errors.New("нужен вход")
)

func protect(next http.Handler, g *auth.Guard) http.Handler {
	if g == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case r.URL.Path == "/api/auth/status" && r.Method == http.MethodGet:
			next.ServeHTTP(w, r)
			return
		case r.URL.Path == "/api/auth/login" && r.Method == http.MethodPost:
			next.ServeHTTP(w, r)
			return
		case r.URL.Path == "/api/auth/setup" && r.Method == http.MethodPost:
			next.ServeHTTP(w, r)
			return
		case r.URL.Path == "/api/auth/logout" && r.Method == http.MethodPost:
			next.ServeHTTP(w, r)
			return
		case r.URL.Path == "/api/health" && r.Method == http.MethodGet:
			if isLoopback(r) || g.Authenticate(tokenFrom(r)) {
				next.ServeHTTP(w, r)
				return
			}
			if !g.PasswordSet() {
				writeErr(w, http.StatusUnauthorized, errNeedPassword)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		if !requestAllowed(g, r) {
			if !g.PasswordSet() {
				writeErr(w, http.StatusUnauthorized, errNeedPassword)
				return
			}
			writeErr(w, http.StatusUnauthorized, errNeedLogin)
			return
		}
		if usesCookie(r) && r.Header.Get("X-Prep-Client") == "" {
			writeErr(w, http.StatusForbidden, errors.New("нужен заголовок X-Prep-Client"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestAllowed(g *auth.Guard, r *http.Request) bool {
	if !g.PasswordSet() {
		return isLoopback(r)
	}
	return g.Authenticate(tokenFrom(r))
}

func tokenFrom(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func usesCookie(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return false
	}
	c, err := r.Cookie(auth.CookieName)
	return err == nil && c.Value != ""
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isLoopback(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	return ip != nil && ip.IsLoopback()
}

func requestSecure(r *http.Request, trustProxy bool) bool {
	if r.TLS != nil {
		return true
	}
	return trustProxy && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, trustProxy bool, token string, maxAge int) {
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestSecure(r, trustProxy),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request, trustProxy bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestSecure(r, trustProxy),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func requireClientHeader(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-Prep-Client") == "" {
		writeErr(w, http.StatusForbidden, errors.New("нужен заголовок X-Prep-Client"))
		return false
	}
	return true
}

func writeAuthErr(w http.ResponseWriter, err error) {
	var rl auth.RateLimitError
	if errors.As(err, &rl) {
		sec := int(rl.RetryAfter.Seconds())
		if sec < 1 {
			sec = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(sec))
		writeErr(w, http.StatusTooManyRequests, err)
		return
	}
	switch {
	case errors.Is(err, auth.ErrBadPassword):
		writeErr(w, http.StatusUnauthorized, err)
	case errors.Is(err, auth.ErrPasswordSet):
		writeErr(w, http.StatusConflict, err)
	case errors.Is(err, auth.ErrNoPassword):
		writeErr(w, http.StatusUnauthorized, errNeedPassword)
	default:
		writeErr(w, http.StatusBadRequest, err)
	}
}

func withSecurity(next http.Handler, trustProxy bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// Flutter web starts from an inline script, so script-src is not locked down.
		// Framing and base-uri are still closed.
		w.Header().Set("Content-Security-Policy", "base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		if requestSecure(r, trustProxy) {
			w.Header().Set("Strict-Transport-Security", "max-age=15552000")
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler, allowed []string) http.Handler {
	allow := make(map[string]struct{}, len(allowed))
	for _, origin := range allowed {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allow[origin] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if _, ok := allow[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Prep-Client")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
