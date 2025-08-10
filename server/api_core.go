package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type api struct {
	store *Store
	log   *slog.Logger
	bus   *EventBus
	// rate limiting buckets per IP:key
	rlMu sync.Mutex
	rl   map[string]*rateBucket
	// dev password reset tokens (in-memory)
	prMu  sync.Mutex
	prTok map[string]resetReq
}

func newAPI(store *Store, log *slog.Logger) *api {
	return &api{store: store, log: log, bus: NewEventBus(), rl: map[string]*rateBucket{}, prTok: map[string]resetReq{}}
}

type rateBucket struct {
	count   int
	resetAt time.Time
}

func (a *api) allow(ip, key string, max int, window time.Duration) bool {
	now := time.Now()
	rk := ip + ":" + key
	a.rlMu.Lock()
	b, ok := a.rl[rk]
	if !ok || now.After(b.resetAt) {
		b = &rateBucket{count: 0, resetAt: now.Add(window)}
		a.rl[rk] = b
	}
	if b.count >= max {
		a.rlMu.Unlock()
		return false
	}
	b.count++
	a.rlMu.Unlock()
	return true
}

func (a *api) withRateLimit(name string, max int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if !a.allow(ip, name, max, window) {
			writeError(w, 429, "too many requests")
			return
		}
		next(w, r)
	}
}

type resetReq struct {
	Email     string
	ExpiresAt time.Time
}

func (a *api) putResetToken(email, token string, ttl time.Duration) {
	a.prMu.Lock()
	defer a.prMu.Unlock()
	a.prTok[token] = resetReq{Email: email, ExpiresAt: time.Now().Add(ttl)}
}
func (a *api) takeResetToken(token string) (string, bool) {
	a.prMu.Lock()
	defer a.prMu.Unlock()
	req, ok := a.prTok[token]
	if !ok {
		return "", false
	}
	if time.Now().After(req.ExpiresAt) {
		delete(a.prTok, token)
		return "", false
	}
	delete(a.prTok, token)
	return req.Email, true
}

func parseID(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, r.Body)
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": msg})
}

// cookie/session helpers
func (a *api) sessionCookieName() string { return getenv("SESSION_COOKIE_NAME", "trellolite_sess") }
func (a *api) sessionTTL() time.Duration {
	if v := getenv("SESSION_TTL", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 14 * 24 * time.Hour
}
func (a *api) secureCookie() bool { return getenv("COOKIE_SECURE", "false") == "true" }
func (a *api) sameSite() http.SameSite {
	switch strings.ToLower(getenv("COOKIE_SAMESITE", "lax")) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (a *api) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.sessionCookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie(),
		SameSite: a.sameSite(),
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}
func (a *api) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.sessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie(),
		SameSite: a.sameSite(),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (a *api) currentUser(r *http.Request) (*User, error) {
	c, err := r.Cookie(a.sessionCookieName())
	if err != nil || c.Value == "" {
		return nil, ErrNotFound
	}
	u, err := a.store.UserBySession(r.Context(), c.Value)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// requireAuth wraps a handler and enforces a valid session
func (a *api) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.currentUser(r); err != nil {
			writeError(w, 401, "unauthorized")
			return
		}
		next(w, r)
	}
}

// --- Admin helper ---
func (a *api) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := a.currentUser(r)
		if err != nil {
			writeError(w, 401, "unauthorized")
			return
		}
		if !u.IsAdmin {
			writeError(w, 403, "forbidden")
			return
		}
		next(w, r)
	}
}

func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(sw, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur_ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

// Implement http.Flusher if underlying writer supports it (needed for SSE)
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
