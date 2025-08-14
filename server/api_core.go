package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"net/smtp"
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
	// dev email verification tokens (in-memory)
	evMu  sync.Mutex
	evTok map[string]verifyReq
}

func newAPI(store *Store, log *slog.Logger) *api {
	return &api{store: store, log: log, bus: NewEventBus(), rl: map[string]*rateBucket{}, prTok: map[string]resetReq{}, evTok: map[string]verifyReq{}}
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

// email verification token storage (dev)
type verifyReq struct {
	Email     string
	ExpiresAt time.Time
}

func (a *api) putVerifyToken(email, token string, ttl time.Duration) {
	a.evMu.Lock()
	defer a.evMu.Unlock()
	a.evTok[token] = verifyReq{Email: email, ExpiresAt: time.Now().Add(ttl)}
}
func (a *api) takeVerifyToken(token string) (string, bool) {
	a.evMu.Lock()
	defer a.evMu.Unlock()
	req, ok := a.evTok[token]
	if !ok {
		return "", false
	}
	if time.Now().After(req.ExpiresAt) {
		delete(a.evTok, token)
		return "", false
	}
	delete(a.evTok, token)
	return req.Email, true
}

// sendEmail sends a plain-text email via SMTP if SMTP_* env vars are configured.
// Required: SMTP_HOST, SMTP_PORT, SMTP_FROM. Optional: SMTP_USERNAME, SMTP_PASSWORD.
func (a *api) sendEmail(to, subject, body string) error {
	host := getenv("SMTP_HOST", "")
	port := getenv("SMTP_PORT", "")
	from := getenv("SMTP_FROM", "")
	if host == "" || port == "" || from == "" {
		return nil // not configured; treat as no-op in dev
	}
	// normalize From: remove wrapping quotes if present in env
	from = strings.Trim(from, "\"'")
	// Parse address to separate envelope sender and header
	var envelopeFrom string
	var headerFrom string
	if addr, err := mail.ParseAddress(from); err == nil {
		envelopeFrom = addr.Address
		headerFrom = addr.String()
	} else {
		envelopeFrom = from
		headerFrom = from
	}

	addr := host + ":" + port
	user := getenv("SMTP_USERNAME", "")
	pass := getenv("SMTP_PASSWORD", "")
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	// Build headers
	subj := mime.BEncoding.Encode("UTF-8", subject)
	date := time.Now().Format(time.RFC1123Z)
	// crude Message-ID using domain part of envelopeFrom if available
	msgIDDomain := "localhost"
	if i := strings.LastIndex(envelopeFrom, "@"); i > 0 && i+1 < len(envelopeFrom) {
		msgIDDomain = envelopeFrom[i+1:]
	}
	msgID := "<" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@" + msgIDDomain + ">"
	msg := "From: " + headerFrom + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subj + "\r\n" +
		"Date: " + date + "\r\n" +
		"Message-ID: " + msgID + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		body + "\r\n"
	err := smtp.SendMail(addr, auth, envelopeFrom, []string{to}, []byte(msg))
	if err != nil {
		a.log.Error("smtp send failed", "err", err)
	}
	if err == nil {
		a.log.Info("smtp sent", "to", to, "subject", subject)
	}
	return err
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
