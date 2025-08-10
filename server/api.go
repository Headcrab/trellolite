package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
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

// --- OAuth helpers (GitHub) ---
func (a *api) githubEnabled() bool {
	return getenv("OAUTH_GITHUB_CLIENT_ID", "") != "" && getenv("OAUTH_GITHUB_CLIENT_SECRET", "") != ""
}

func (a *api) setStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie(),
		SameSite: a.sameSite(),
		Expires:  time.Now().Add(5 * time.Minute),
		MaxAge:   300,
	})
}

func (a *api) readStateCookie(r *http.Request) (string, error) {
	c, err := r.Cookie("oauth_state")
	if err != nil {
		return "", err
	}
	return c.Value, nil
}

// Auth handlers
func (a *api) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password, Name string }
	if err := readJSON(w, r, &req); err != nil || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		writeError(w, 400, "invalid payload")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, 400, "password too short")
		return
	}
	// hash
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		a.log.Error("bcrypt", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	u, err := a.store.CreateUser(r.Context(), req.Email, string(hashBytes), strings.TrimSpace(req.Name))
	if err != nil {
		a.log.Error("register", "err", err)
		writeError(w, 400, "cannot create user")
		return
	}
	// session
	token, exp, err := a.store.CreateSession(r.Context(), u.ID, a.sessionTTL())
	if err != nil {
		a.log.Error("create session", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	a.setSessionCookie(w, token, exp)
	writeJSON(w, 201, map[string]any{"ok": true, "user": u})
}

func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password string }
	if err := readJSON(w, r, &req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, 400, "invalid payload")
		return
	}
	u, err := a.store.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, 401, "invalid credentials")
		return
	}
	token, exp, err := a.store.CreateSession(r.Context(), u.ID, a.sessionTTL())
	if err != nil {
		a.log.Error("create session", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	a.setSessionCookie(w, token, exp)
	writeJSON(w, 200, map[string]any{"ok": true, "user": u})
}

func (a *api) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(a.sessionCookieName()); err == nil && c.Value != "" {
		_ = a.store.DeleteSession(r.Context(), c.Value)
	}
	a.clearSessionCookie(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		// For anonymous users return 200 with user: null to avoid noisy 401s on public pages
		writeJSON(w, 200, map[string]any{"user": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}

// Providers list for UI
func (a *api) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	providers := []map[string]string{}
	if a.githubEnabled() {
		providers = append(providers, map[string]string{"id": "github", "name": "GitHub"})
	}
	writeJSON(w, 200, map[string]any{"providers": providers})
}

// GET /api/auth/oauth/github/start
func (a *api) handleGithubStart(w http.ResponseWriter, r *http.Request) {
	if !a.githubEnabled() {
		writeError(w, 404, "provider not configured")
		return
	}
	// generate state
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := base64RawURL(b)
	a.setStateCookie(w, state)
	clientID := getenv("OAUTH_GITHUB_CLIENT_ID", "")
	redirectURI := getenv("OAUTH_GITHUB_REDIRECT_URL", "")
	if redirectURI == "" {
		// best-effort default for local dev
		scheme := "http"
		host := r.Host
		redirectURI = scheme + "://" + host + "/api/auth/oauth/github/callback"
	}
	u, _ := url.Parse("https://github.com/login/oauth/authorize")
	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// GET /api/auth/oauth/github/callback
func (a *api) handleGithubCallback(w http.ResponseWriter, r *http.Request) {
	if !a.githubEnabled() {
		writeError(w, 404, "provider not configured")
		return
	}
	qs := r.URL.Query()
	code := qs.Get("code")
	st := qs.Get("state")
	if code == "" || st == "" {
		writeError(w, 400, "bad oauth response")
		return
	}
	if have, err := a.readStateCookie(r); err != nil || have == "" || have != st {
		writeError(w, 400, "state mismatch")
		return
	}
	token, err := a.githubExchangeToken(r.Context(), code)
	if err != nil {
		a.log.Error("oauth token", "err", err)
		writeError(w, 502, "oauth error")
		return
	}
	gh, email, err := a.githubFetchUser(r.Context(), token)
	if err != nil {
		a.log.Error("oauth user", "err", err)
		writeError(w, 502, "oauth error")
		return
	}
	if email == "" { // fallback to noreply if email hidden
		email = "github-" + gh.ID + "@users.noreply.github.com"
	}
	name := strings.TrimSpace(gh.Name)
	if name == "" {
		name = gh.Login
	}
	u, err := a.store.EnsureOAuthUser(r.Context(), "github", gh.ID, email, name)
	if err != nil {
		a.log.Error("ensure oauth user", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	tok, exp, err := a.store.CreateSession(r.Context(), u.ID, a.sessionTTL())
	if err != nil {
		a.log.Error("create session", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	a.setSessionCookie(w, tok, exp)
	http.Redirect(w, r, "/", http.StatusFound)
}

// --- GitHub API calls ---
func base64RawURL(b []byte) string {
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	// manual to avoid bringing encoding/base64 if not desired, but we can use stdlib
	// Using stdlib for simplicity
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b), "=")
}

type ghUser struct{ ID, Login, Name string }

func (a *api) githubExchangeToken(ctx context.Context, code string) (string, error) {
	data := url.Values{}
	data.Set("client_id", getenv("OAUTH_GITHUB_CLIENT_ID", ""))
	data.Set("client_secret", getenv("OAUTH_GITHUB_CLIENT_SECRET", ""))
	ru := getenv("OAUTH_GITHUB_REDIRECT_URL", "")
	if ru != "" {
		data.Set("redirect_uri", ru)
	}
	data.Set("code", code)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", bytes.NewBufferString(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != "" || out.AccessToken == "" {
		return "", errors.New("oauth token error")
	}
	return out.AccessToken, nil
}

func (a *api) githubFetchUser(ctx context.Context, token string) (ghUser, string, error) {
	// /user
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ghUser{}, "", err
	}
	defer resp.Body.Close()
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return ghUser{}, "", err
	}
	gh := ghUser{ID: strconv.FormatInt(u.ID, 10), Login: u.Login, Name: u.Name}
	// /user/emails to get verified primary email
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	req2.Header.Set("Accept", "application/vnd.github+json")
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return gh, "", nil
	}
	defer resp2.Body.Close()
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&emails); err != nil {
		return gh, "", nil
	}
	var chosen string
	for _, e := range emails {
		if e.Primary && e.Verified {
			chosen = e.Email
			break
		}
	}
	if chosen == "" {
		for _, e := range emails {
			if e.Verified {
				chosen = e.Email
				break
			}
		}
	}
	if chosen == "" && len(emails) > 0 {
		chosen = emails[0].Email
	}
	return gh, chosen, nil
}

func (a *api) routes(mux *http.ServeMux) {
	// Auth endpoints
	mux.HandleFunc("POST /api/auth/register", a.withRateLimit("auth", 20, time.Minute, a.handleRegister))
	mux.HandleFunc("POST /api/auth/login", a.withRateLimit("auth", 30, time.Minute, a.handleLogin))
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)
	mux.HandleFunc("GET /api/auth/providers", a.handleAuthProviders)
	mux.HandleFunc("GET /api/auth/oauth/github/start", a.handleGithubStart)
	mux.HandleFunc("GET /api/auth/oauth/github/callback", a.handleGithubCallback)

	// Dev password reset (magic link in logs)
	mux.HandleFunc("POST /api/auth/reset", a.withRateLimit("auth_reset", 10, time.Minute, a.handleResetRequest))
	mux.HandleFunc("POST /api/auth/reset/confirm", a.withRateLimit("auth_reset", 20, time.Minute, a.handleResetConfirm))

	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/boards", a.handleListBoards)
	mux.HandleFunc("POST /api/boards", a.requireAuth(a.handleCreateBoard))
	mux.HandleFunc("GET /api/boards/{id}", a.requireAuth(a.handleGetBoard))
	mux.HandleFunc("GET /api/boards/{id}/full", a.requireAuth(a.handleGetBoardFull))
	mux.HandleFunc("GET /api/boards/{id}/events", a.requireAuth(a.handleBoardEvents))
	mux.HandleFunc("PATCH /api/boards/{id}", a.requireAuth(a.handleUpdateBoard))
	mux.HandleFunc("POST /api/boards/{id}/move", a.requireAuth(a.handleMoveBoard))
	mux.HandleFunc("DELETE /api/boards/{id}", a.requireAuth(a.handleDeleteBoard))

	mux.HandleFunc("GET /api/boards/{id}/lists", a.requireAuth(a.handleListsByBoard))
	mux.HandleFunc("POST /api/boards/{id}/lists", a.requireAuth(a.handleCreateList))
	mux.HandleFunc("PATCH /api/lists/{id}", a.requireAuth(a.handleUpdateList))
	mux.HandleFunc("POST /api/lists/{id}/move", a.requireAuth(a.handleMoveList))
	mux.HandleFunc("DELETE /api/lists/{id}", a.requireAuth(a.handleDeleteList))

	mux.HandleFunc("GET /api/lists/{id}/cards", a.requireAuth(a.handleCardsByList))
	mux.HandleFunc("POST /api/lists/{id}/cards", a.requireAuth(a.handleCreateCard))
	mux.HandleFunc("PATCH /api/cards/{id}", a.requireAuth(a.handleUpdateCard))
	mux.HandleFunc("DELETE /api/cards/{id}", a.requireAuth(a.handleDeleteCard))
	mux.HandleFunc("POST /api/cards/{id}/move", a.requireAuth(a.handleMoveCard))

	mux.HandleFunc("GET /api/cards/{id}/comments", a.requireAuth(a.handleCommentsByCard))
	mux.HandleFunc("POST /api/cards/{id}/comments", a.requireAuth(a.handleAddComment))

	// Groups and board visibility
	mux.HandleFunc("POST /api/groups", a.requireAuth(a.handleCreateGroupSelf))
	mux.HandleFunc("GET /api/my/groups", a.requireAuth(a.handleMyGroups))
	// Self-managed groups (creators/admins)
	mux.HandleFunc("GET /api/groups/{id}/users", a.requireAuth(a.handleSelfGroupUsers))
	mux.HandleFunc("GET /api/groups/{id}/users/search", a.requireAuth(a.handleSelfSearchUsers))
	mux.HandleFunc("POST /api/groups/{id}/users", a.requireAuth(a.handleSelfAddUserToGroup))
	mux.HandleFunc("DELETE /api/groups/{id}/users/{uid}", a.requireAuth(a.handleSelfRemoveUserFromGroup))
	// Self leave from a group (any member can leave)
	mux.HandleFunc("POST /api/groups/{id}/leave", a.requireAuth(a.handleSelfLeaveGroup))
	mux.HandleFunc("DELETE /api/groups/{id}", a.requireAuth(a.handleSelfDeleteGroup))
	mux.HandleFunc("GET /api/boards/{id}/groups", a.requireAuth(a.handleBoardGroups))
	mux.HandleFunc("POST /api/boards/{id}/groups", a.requireAuth(a.handleBoardGroupAdd))
	mux.HandleFunc("DELETE /api/boards/{id}/groups/{gid}", a.requireAuth(a.handleBoardGroupRemove))

	// Admin: groups CRUD and membership
	mux.HandleFunc("GET /api/admin/groups", a.requireAdmin(a.handleAdminListGroups))
	mux.HandleFunc("POST /api/admin/groups", a.requireAdmin(a.handleAdminCreateGroup))
	mux.HandleFunc("DELETE /api/admin/groups/{id}", a.requireAdmin(a.handleAdminDeleteGroup))
	mux.HandleFunc("GET /api/admin/groups/{id}/users", a.requireAdmin(a.handleAdminGroupUsers))
	mux.HandleFunc("POST /api/admin/groups/{id}/users", a.requireAdmin(a.handleAdminAddUserToGroup))
	mux.HandleFunc("DELETE /api/admin/groups/{id}/users/{uid}", a.requireAdmin(a.handleAdminRemoveUserFromGroup))
	mux.HandleFunc("GET /api/admin/users", a.requireAdmin(a.handleAdminListUsers))

	// Projects
	mux.HandleFunc("GET /api/projects", a.requireAuth(a.handleListProjects))
	mux.HandleFunc("POST /api/projects", a.requireAuth(a.handleCreateProject))
	mux.HandleFunc("GET /api/projects/{id}/members", a.requireAuth(a.handleProjectMembers))
	mux.HandleFunc("POST /api/projects/{id}/members", a.requireAuth(a.handleAddProjectMember))
	mux.HandleFunc("DELETE /api/projects/{id}/members/{uid}", a.requireAuth(a.handleRemoveProjectMember))
}

// POST /api/groups {name}
func (a *api) handleCreateGroupSelf(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if e := readJSON(w, r, &req); e != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, 400, "invalid payload")
		return
	}
	g, e := a.store.CreateGroupOwned(r.Context(), u.ID, strings.TrimSpace(req.Name))
	if e != nil {
		a.log.Error("create group self", "err", e)
		writeError(w, 400, "cannot create group")
		return
	}
	writeJSON(w, 201, g)
}

// POST /api/auth/reset {email}
func (a *api) handleResetRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(w, r, &req); err != nil || strings.TrimSpace(req.Email) == "" {
		writeError(w, 400, "invalid payload")
		return
	}
	email := strings.TrimSpace(req.Email)
	// Generate token regardless of user existence (no enumeration)
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	a.putResetToken(email, tok, 15*time.Minute)
	// Log dev magic-link
	host := r.Host
	a.log.Info("password reset link (dev)", "email", email, "token", tok, "url", fmt.Sprintf("http://%s/web/login.html#reset=%s", host, tok))
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/auth/reset/confirm {token, new_password}
func (a *api) handleResetConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token, NewPassword string }
	if err := readJSON(w, r, &req); err != nil || strings.TrimSpace(req.Token) == "" || len(req.NewPassword) < 6 {
		writeError(w, 400, "invalid payload")
		return
	}
	email, ok := a.takeResetToken(strings.TrimSpace(req.Token))
	if !ok {
		writeError(w, 400, "invalid token")
		return
	}
	// Update password if user exists; if not, no-op for privacy
	if strings.TrimSpace(email) != "" {
		if err := a.store.UpdateUserPasswordByEmail(r.Context(), email, req.NewPassword); err != nil {
			a.log.Error("reset password", "err", err)
			writeError(w, 500, "internal error")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
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

func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "ts": time.Now().UTC().Format(time.RFC3339)})
}

func (a *api) handleListBoards(w http.ResponseWriter, r *http.Request) {
	// Only authenticated users can list boards; apply scope filter
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "mine"
	}
	items, err := a.store.ListBoards(r.Context(), u.ID, scope)
	if err != nil {
		a.log.Error("list boards", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}
func (a *api) handleCreateBoard(w http.ResponseWriter, r *http.Request) {
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	var req struct {
		Title     string `json:"title"`
		ProjectID int64  `json:"project_id"`
	}
	if err := readJSON(w, r, &req); err != nil || len(req.Title) == 0 {
		if err != nil {
			a.log.Error("decode create board", "err", err)
		}
		writeError(w, 400, "invalid payload")
		return
	}
	b, err := a.store.CreateBoard(r.Context(), u.ID, req.Title)
	if err != nil {
		a.log.Error("create board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	if req.ProjectID != 0 {
		// Only allow if user has access to this project
		if ok, _ := a.store.CanAccessProject(r.Context(), req.ProjectID, u.ID); ok {
			_ = a.store.SetBoardProject(r.Context(), b.ID, req.ProjectID)
		}
	}
	writeJSON(w, 201, b)
}

// --- Projects endpoints ---
func (a *api) handleListProjects(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	items, e := a.store.ListProjects(r.Context(), u.ID)
	if e != nil {
		a.log.Error("list projects", "err", e)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}

func (a *api) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if e := readJSON(w, r, &req); e != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, 400, "invalid payload")
		return
	}
	p, e := a.store.CreateProject(r.Context(), u.ID, strings.TrimSpace(req.Name))
	if e != nil {
		a.log.Error("create project", "err", e)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, p)
}

func (a *api) handleProjectMembers(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	id, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	if ok, e := a.store.CanAccessProject(r.Context(), id, u.ID); e != nil || !ok {
		writeError(w, 403, "forbidden")
		return
	}
	items, e2 := a.store.ProjectMembers(r.Context(), id)
	if e2 != nil {
		a.log.Error("project members", "err", e2)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}

func (a *api) handleAddProjectMember(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	id, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	// Only project owner can add
	if own, e := a.store.IsProjectOwner(r.Context(), id, u.ID); e != nil || !own {
		writeError(w, 403, "forbidden")
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
		Role   int   `json:"role"`
	}
	if e := readJSON(w, r, &req); e != nil || req.UserID == 0 {
		writeError(w, 400, "invalid payload")
		return
	}
	if e := a.store.AddProjectMember(r.Context(), id, req.UserID, req.Role); e != nil {
		a.log.Error("add proj member", "err", e)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleRemoveProjectMember(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	id, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	uid, e := parseID(r.PathValue("uid"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	// Only project owner can remove
	if own, e := a.store.IsProjectOwner(r.Context(), id, u.ID); e != nil || !own {
		writeError(w, 403, "forbidden")
		return
	}
	if e := a.store.RemoveProjectMember(r.Context(), id, uid); e != nil {
		a.log.Error("rm proj member", "err", e)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (a *api) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	// Gate by access: owner or group member (auth required by wrapper)
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	ok, e := a.store.CanAccessBoard(r.Context(), u.ID, id)
	if e != nil {
		a.log.Error("access check", "err", e)
	}
	if !ok {
		writeError(w, 403, "forbidden")
		return
	}
	b, err := a.store.GetBoard(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("get board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, b)
}
func (a *api) handleUpdateBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	// Only owner can update board properties
	if own, e := a.store.IsBoardOwner(r.Context(), id, u.ID); e != nil || !own {
		writeError(w, 403, "forbidden")
		return
	}
	var req struct {
		Title *string `json:"title"`
		Color *string `json:"color"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeError(w, 400, "title cannot be empty")
			return
		}
		if err := a.store.UpdateBoard(r.Context(), id, title); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeError(w, 404, "not found")
				return
			}
			a.log.Error("update board", "err", err)
			writeError(w, 500, "internal error")
			return
		}
	}
	if req.Color != nil {
		if _, err := a.store.db.ExecContext(r.Context(), `update boards set color=$1 where id=$2`, *req.Color, id); err != nil {
			a.log.Error("update board color", "err", err)
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	// publish update event
	a.bus.Publish(Event{Type: "board.updated", Entity: "board", BoardID: id, Payload: map[string]any{"id": id}})
}
func (a *api) handleDeleteBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	// Only owner can delete
	if own, e := a.store.IsBoardOwner(r.Context(), id, u.ID); e != nil || !own {
		writeError(w, 403, "forbidden")
		return
	}
	if err := a.store.DeleteBoard(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("delete board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleMoveBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if own, e := a.store.IsBoardOwner(r.Context(), id, u.ID); e != nil || !own {
		writeError(w, 403, "forbidden")
		return
	}
	var req struct {
		NewIndex int `json:"new_index"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	if err := a.store.MoveBoard(r.Context(), id, req.NewIndex); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("move board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	a.bus.Publish(Event{Type: "board.moved", Entity: "board", BoardID: id, Payload: map[string]any{"id": id, "new_index": req.NewIndex}})
}

func (a *api) handleListsByBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	ok, e := a.store.CanAccessBoard(r.Context(), u.ID, id)
	if e != nil {
		a.log.Error("access check", "err", e)
	}
	if !ok {
		writeError(w, 403, "forbidden")
		return
	}
	items, err := a.store.ListsByBoard(r.Context(), id)
	if err != nil {
		a.log.Error("lists by board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}
func (a *api) handleCreateList(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := readJSON(w, r, &req); err != nil || len(req.Title) == 0 {
		if err != nil {
			a.log.Error("decode create list", "err", err)
		}
		writeError(w, 400, "invalid payload")
		return
	}
	l, err := a.store.CreateList(r.Context(), id, req.Title)
	if err != nil {
		a.log.Error("create list", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, l)
	// publish list created
	a.bus.Publish(Event{Type: "list.created", Entity: "list", BoardID: l.BoardID, ListID: &l.ID, Payload: l})
}
func (a *api) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct {
		Title *string `json:"title"`
		Pos   *int64  `json:"pos"`
		Color *string `json:"color"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	if err := a.store.UpdateList(r.Context(), id, req.Title, req.Pos); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("update list", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	if req.Color != nil {
		if _, err := a.store.db.ExecContext(r.Context(), `update lists set color=$1 where id=$2`, *req.Color, id); err != nil {
			a.log.Error("update list color", "err", err)
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	if bid, e := a.store.BoardIDByList(r.Context(), id); e == nil {
		aID := id
		a.bus.Publish(Event{Type: "list.updated", Entity: "list", BoardID: bid, ListID: &aID, Payload: map[string]any{"id": id}})
	}
}
func (a *api) handleDeleteList(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.DeleteList(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("delete list", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	if bid, e := a.store.BoardIDByList(r.Context(), id); e == nil {
		aID := id
		a.bus.Publish(Event{Type: "list.deleted", Entity: "list", BoardID: bid, ListID: &aID, Payload: map[string]any{"id": id}})
	}
}

func (a *api) handleMoveList(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct {
		NewIndex      int   `json:"new_index"`
		TargetBoardID int64 `json:"target_board_id"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	// detect source board to emit events
	var srcBid int64
	if bid, e := a.store.BoardIDByList(r.Context(), id); e == nil {
		srcBid = bid
	}
	if err := a.store.MoveList(r.Context(), id, req.TargetBoardID, req.NewIndex); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("move list", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	if dstBid, e := a.store.BoardIDByList(r.Context(), id); e == nil {
		// notify destination board
		a.bus.Publish(Event{Type: "list.moved", Entity: "list", BoardID: dstBid, ListID: &id, Payload: map[string]any{"id": id, "new_index": req.NewIndex}})
		// if moved across boards, notify source board about deletion of this list
		if srcBid != 0 && srcBid != dstBid {
			a.bus.Publish(Event{Type: "list.deleted", Entity: "list", BoardID: srcBid, ListID: &id, Payload: map[string]any{"id": id}})
		}
	}
}

func (a *api) handleCardsByList(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if bid, e := a.store.BoardIDByList(r.Context(), id); e == nil {
		ok, e2 := a.store.CanAccessBoard(r.Context(), u.ID, bid)
		if e2 != nil {
			a.log.Error("access check", "err", e2)
		}
		if !ok {
			writeError(w, 403, "forbidden")
			return
		}
	}
	items, err := a.store.CardsByList(r.Context(), id)
	if err != nil {
		a.log.Error("cards by list", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}
func (a *api) handleCreateCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct{ Title, Description string }
	if err := readJSON(w, r, &req); err != nil || len(req.Title) == 0 {
		if err != nil {
			a.log.Error("decode create card", "err", err)
		}
		writeError(w, 400, "invalid payload")
		return
	}
	c, err := a.store.CreateCard(r.Context(), id, req.Title, req.Description)
	if err != nil {
		a.log.Error("create card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, c)
	// Publish on card create
	if bid, e := a.store.BoardIDByList(r.Context(), c.ListID); e == nil {
		a.bus.Publish(Event{Type: "card.created", Entity: "card", BoardID: bid, ListID: &c.ListID, Payload: c})
	}
}
func (a *api) handleUpdateCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var due *time.Time
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Pos         *int64  `json:"pos"`
		DueAt       *string `json:"due_at"`
		Color       *string `json:"color"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	if req.DueAt != nil && *req.DueAt != "" {
		if t, e := time.Parse(time.RFC3339, *req.DueAt); e == nil {
			due = &t
		}
	}
	if err := a.store.UpdateCard(r.Context(), id, req.Title, req.Description, req.Pos, due); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("update card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	if req.Color != nil {
		if _, err := a.store.db.ExecContext(r.Context(), `update cards set color=$1 where id=$2`, *req.Color, id); err != nil {
			a.log.Error("update card color", "err", err)
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	if bid, _, e := a.store.BoardAndListByCard(r.Context(), id); e == nil {
		a.bus.Publish(Event{Type: "card.updated", Entity: "card", BoardID: bid, Payload: map[string]any{"id": id}})
	}
}
func (a *api) handleDeleteCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	res, err := a.store.db.ExecContext(r.Context(), `delete from cards where id=$1`, id)
	if err != nil {
		a.log.Error("delete card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	if bid, _, e := a.store.BoardAndListByCard(r.Context(), id); e == nil {
		a.bus.Publish(Event{Type: "card.deleted", Entity: "card", BoardID: bid, Payload: map[string]any{"id": id}})
	}
}
func (a *api) handleMoveCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct {
		TargetListID int64 `json:"target_list_id"`
		NewIndex     int   `json:"new_index"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	if err := a.store.MoveCard(r.Context(), id, req.TargetListID, req.NewIndex); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("move card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	if bid, _, e := a.store.BoardAndListByCard(r.Context(), id); e == nil {
		a.bus.Publish(Event{Type: "card.moved", Entity: "card", BoardID: bid, Payload: map[string]any{"id": id, "target_list_id": req.TargetListID, "new_index": req.NewIndex}})
	}
}
func (a *api) handleAddComment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := readJSON(w, r, &req); err != nil || len(req.Body) == 0 {
		writeError(w, 400, "invalid payload")
		return
	}
	c, err := a.store.AddComment(r.Context(), id, req.Body)
	if err != nil {
		a.log.Error("add comment", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, c)
	if bid, lid, e := a.store.BoardAndListByCard(r.Context(), id); e == nil {
		a.bus.Publish(Event{Type: "comment.created", Entity: "comment", BoardID: bid, ListID: &lid, Payload: c})
	}
}
func (a *api) handleCommentsByCard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	// authorize via board access from card
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if bid, _, e := a.store.BoardAndListByCard(r.Context(), id); e == nil {
		ok, e2 := a.store.CanAccessBoard(r.Context(), u.ID, bid)
		if e2 != nil {
			a.log.Error("access check", "err", e2)
		}
		if !ok {
			writeError(w, 403, "forbidden")
			return
		}
	}
	items, err := a.store.CommentsByCard(r.Context(), id)
	if err != nil {
		a.log.Error("comments by card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}

// --- Admin helpers and handlers ---
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

func (a *api) handleAdminListGroups(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListGroups(r.Context())
	if err != nil {
		a.log.Error("admin list groups", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}

func (a *api) handleAdminCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, 400, "invalid payload")
		return
	}
	g, err := a.store.CreateGroup(r.Context(), strings.TrimSpace(req.Name))
	if err != nil {
		a.log.Error("admin create group", "err", err)
		writeError(w, 400, "cannot create group")
		return
	}
	writeJSON(w, 201, g)
}

func (a *api) handleAdminDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.DeleteGroup(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("admin delete group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleAdminGroupUsers(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	users, err := a.store.GroupUsers(r.Context(), id)
	if err != nil {
		a.log.Error("admin group users", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, users)
}

func (a *api) handleAdminAddUserToGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := readJSON(w, r, &req); err != nil || req.UserID == 0 {
		writeError(w, 400, "invalid payload")
		return
	}
	if err := a.store.AddUserToGroup(r.Context(), id, req.UserID); err != nil {
		a.log.Error("admin add user to group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleAdminRemoveUserFromGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	uid, err := parseID(r.PathValue("uid"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.RemoveUserFromGroup(r.Context(), id, uid); err != nil {
		a.log.Error("admin remove user from group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	items, err := a.store.ListUsers(r.Context(), q, limit)
	if err != nil {
		a.log.Error("admin list users", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}

// --- Groups & board visibility ---
func (a *api) handleMyGroups(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	groups, err := a.store.MyGroups(r.Context(), u.ID)
	if err != nil {
		a.log.Error("my groups", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, groups)
}

// --- Self-managed groups ---
func (a *api) handleSelfGroupUsers(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	gid, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	if ok, e2 := a.store.IsGroupAdmin(r.Context(), gid, u.ID); e2 != nil || !ok {
		writeError(w, 403, "forbidden")
		return
	}
	users, err := a.store.GroupUsersForSelf(r.Context(), gid)
	if err != nil {
		a.log.Error("self group users", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, users)
}

func (a *api) handleSelfAddUserToGroup(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	gid, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	if ok, e2 := a.store.IsGroupAdmin(r.Context(), gid, u.ID); e2 != nil || !ok {
		writeError(w, 403, "forbidden")
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if e := readJSON(w, r, &req); e != nil || req.UserID == 0 {
		writeError(w, 400, "invalid payload")
		return
	}
	if err := a.store.AddUserToGroupSelf(r.Context(), gid, req.UserID); err != nil {
		a.log.Error("self add user to group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleSelfSearchUsers(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	gid, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	if ok, e2 := a.store.IsGroupAdmin(r.Context(), gid, u.ID); e2 != nil || !ok {
		writeError(w, 403, "forbidden")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			limit = n
		}
	}
	items, e2 := a.store.ListUsers(r.Context(), q, limit)
	if e2 != nil {
		a.log.Error("self search users", "err", e2)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}

func (a *api) handleSelfRemoveUserFromGroup(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	gid, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	uid, e := parseID(r.PathValue("uid"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	if ok, e2 := a.store.IsGroupAdmin(r.Context(), gid, u.ID); e2 != nil || !ok {
		writeError(w, 403, "forbidden")
		return
	}
	if err := a.store.RemoveUserFromGroupSelf(r.Context(), gid, uid); err != nil {
		a.log.Error("self rm user group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleSelfDeleteGroup(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	gid, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.DeleteGroupIfAdmin(r.Context(), gid, u.ID); err != nil {
		if err.Error() == "forbidden" {
			writeError(w, 403, "forbidden")
			return
		}
		a.log.Error("self delete group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/groups/{id}/leave -- current user leaves the group (if member)
func (a *api) handleSelfLeaveGroup(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	gid, e := parseID(r.PathValue("id"))
	if e != nil {
		writeError(w, 400, "bad id")
		return
	}
	// Remove membership if exists; admins may also leave via this endpoint
	if err := a.store.RemoveUserFromGroup(r.Context(), gid, u.ID); err != nil {
		a.log.Error("self leave group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleBoardGroups(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	// Only board owner may view/manage groups mapping
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	if own, e := a.store.IsBoardOwner(r.Context(), id, u.ID); e != nil || !own {
		writeError(w, 403, "forbidden")
		return
	}
	groups, err := a.store.BoardGroups(r.Context(), id)
	if err != nil {
		a.log.Error("board groups", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, groups)
}

func (a *api) handleBoardGroupAdd(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	var req struct {
		GroupID int64 `json:"group_id"`
	}
	if err := readJSON(w, r, &req); err != nil || req.GroupID == 0 {
		writeError(w, 400, "invalid payload")
		return
	}
	own, err := a.store.IsBoardOwner(r.Context(), id, u.ID)
	if err != nil {
		a.log.Error("check owner", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	if !own {
		writeError(w, 403, "forbidden")
		return
	}
	if err := a.store.AddBoardToGroup(r.Context(), id, req.GroupID); err != nil {
		a.log.Error("add board group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *api) handleBoardGroupRemove(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	gid, err := parseID(r.PathValue("gid"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	own, err := a.store.IsBoardOwner(r.Context(), id, u.ID)
	if err != nil {
		a.log.Error("check owner", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	if !own {
		writeError(w, 403, "forbidden")
		return
	}
	if err := a.store.RemoveBoardFromGroup(r.Context(), id, gid); err != nil {
		a.log.Error("remove board group", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// SSE endpoint for a board
func (a *api) handleBoardEvents(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	if u, errU := a.currentUser(r); errU == nil {
		ok, e := a.store.CanAccessBoard(r.Context(), u.ID, id)
		if e != nil {
			a.log.Error("access check", "err", e)
		}
		if !ok {
			writeError(w, 403, "forbidden")
			return
		}
	}
	a.bus.ServeSSE(w, r, id)
}

// Aggregate endpoint: board with lists and cards
func (a *api) handleGetBoardFull(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	// Gate by access
	if u, errU := a.currentUser(r); errU == nil {
		ok, e := a.store.CanAccessBoard(r.Context(), u.ID, id)
		if e != nil {
			a.log.Error("access check", "err", e)
		}
		if !ok {
			writeError(w, 403, "forbidden")
			return
		}
	}
	board, err := a.store.GetBoard(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("get board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	lists, err := a.store.ListsByBoard(r.Context(), id)
	if err != nil {
		a.log.Error("lists by board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	out := map[string]any{"board": board, "lists": lists, "cards": map[int64][]Card{}}
	cardsMap := out["cards"].(map[int64][]Card)
	for _, l := range lists {
		cards, err := a.store.CardsByList(r.Context(), l.ID)
		if err != nil {
			a.log.Error("cards by list", "err", err)
			writeError(w, 500, "internal error")
			return
		}
		cardsMap[l.ID] = cards
	}
	writeJSON(w, 200, out)
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
