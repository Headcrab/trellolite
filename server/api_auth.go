package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// --- OAuth helpers (GitHub) ---
func (a *api) githubEnabled() bool {
	return getenv("OAUTH_GITHUB_CLIENT_ID", "") != "" && getenv("OAUTH_GITHUB_CLIENT_SECRET", "") != ""
}

// --- OAuth helpers (Google) ---
func (a *api) googleEnabled() bool {
	return getenv("OAUTH_GOOGLE_CLIENT_ID", "") != "" && getenv("OAUTH_GOOGLE_CLIENT_SECRET", "") != ""
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
	if a.googleEnabled() {
		providers = append(providers, map[string]string{"id": "google", "name": "Google"})
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
	// Prefer current request's host to avoid cookie domain mismatch loops
	scheme := "http"
	if r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil {
		scheme = "https"
	}
	ruEnv := getenv("OAUTH_GITHUB_REDIRECT_URL", "")
	redirectURI := ruEnv
	if redirectURI == "" {
		redirectURI = scheme + "://" + r.Host + "/api/auth/oauth/github/callback"
	} else {
		if u2, err := url.Parse(redirectURI); err == nil {
			if !strings.EqualFold(u2.Host, r.Host) {
				a.log.Info("oauth redirect host adjusted to request host", "from", u2.Host, "to", r.Host)
				redirectURI = scheme + "://" + r.Host + "/api/auth/oauth/github/callback"
			}
		}
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
	// Clear used state cookie to prevent stale state causing loops
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie(),
		SameSite: a.sameSite(),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
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
	// Redirect to home after successful OAuth login
	http.Redirect(w, r, "/", http.StatusFound)
}

// --- Google OAuth ---
// GET /api/auth/oauth/google/start
func (a *api) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if !a.googleEnabled() {
		writeError(w, 404, "provider not configured")
		return
	}
	// state
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := base64RawURL(b)
	a.setStateCookie(w, state)

	clientID := getenv("OAUTH_GOOGLE_CLIENT_ID", "")
	// Build redirect URI, prefer request host to avoid domain mismatches
	scheme := "http"
	if r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil {
		scheme = "https"
	}
	ruEnv := getenv("OAUTH_GOOGLE_REDIRECT_URL", "")
	redirectURI := ruEnv
	if redirectURI == "" {
		redirectURI = scheme + "://" + r.Host + "/api/auth/oauth/google/callback"
	} else {
		if u2, err := url.Parse(redirectURI); err == nil {
			if !strings.EqualFold(u2.Host, r.Host) {
				a.log.Info("oauth redirect host adjusted to request host", "from", u2.Host, "to", r.Host)
				redirectURI = scheme + "://" + r.Host + "/api/auth/oauth/google/callback"
			}
		}
	}
	u, _ := url.Parse("https://accounts.google.com/o/oauth2/v2/auth")
	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	// Optional UX tweaks: force consent only if needed; we skip access_type/offline
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// GET /api/auth/oauth/google/callback
func (a *api) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if !a.googleEnabled() {
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
	// Clear used state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie(),
		SameSite: a.sameSite(),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	token, err := a.googleExchangeToken(r.Context(), code, r)
	if err != nil {
		a.log.Error("oauth token", "err", err)
		writeError(w, 502, "oauth error")
		return
	}
	gu, err := a.googleFetchUser(r.Context(), token)
	if err != nil {
		a.log.Error("oauth user", "err", err)
		writeError(w, 502, "oauth error")
		return
	}
	email := gu.Email
	if email == "" {
		// Fallback: synthetic email based on sub
		email = "google-" + gu.Sub + "@users.noreply.google.com"
	}
	name := strings.TrimSpace(gu.Name)
	if name == "" {
		name = email
	}
	u, err := a.store.EnsureOAuthUser(r.Context(), "google", gu.Sub, email, name)
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

type googleUser struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func (a *api) googleExchangeToken(ctx context.Context, code string, r *http.Request) (string, error) {
	// Build redirect URI same as in start
	scheme := "http"
	if r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil {
		scheme = "https"
	}
	ru := getenv("OAUTH_GOOGLE_REDIRECT_URL", "")
	redirectURI := ru
	if redirectURI == "" {
		redirectURI = scheme + "://" + r.Host + "/api/auth/oauth/google/callback"
	}
	data := url.Values{}
	data.Set("client_id", getenv("OAUTH_GOOGLE_CLIENT_ID", ""))
	data.Set("client_secret", getenv("OAUTH_GOOGLE_CLIENT_SECRET", ""))
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", bytes.NewBufferString(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

func (a *api) googleFetchUser(ctx context.Context, token string) (googleUser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return googleUser{}, err
	}
	defer resp.Body.Close()
	var gu googleUser
	if err := json.NewDecoder(resp.Body).Decode(&gu); err != nil {
		return googleUser{}, err
	}
	return gu, nil
}

// --- GitHub API calls ---
func base64RawURL(b []byte) string {
	const _ = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
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
	a.log.Info("password reset link (dev)", "email", email, "token", tok, "url", "http://"+host+"/web/login.html#reset="+tok)
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
