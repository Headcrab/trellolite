package main

import (
	"net/http"
	"strconv"
	"strings"
)

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
		if err == ErrNotFound {
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

// GET /api/admin/system
// Returns basic system capabilities/config flags for admin Settings UI
func (a *api) handleAdminSystemStatus(w http.ResponseWriter, r *http.Request) {
	smtpConfigured := getenv("SMTP_HOST", "") != "" && getenv("SMTP_PORT", "") != "" && getenv("SMTP_FROM", "") != ""
	writeJSON(w, 200, map[string]any{
		"oauth": map[string]bool{
			"github": a.githubEnabled(),
			"google": a.googleEnabled(),
		},
		"smtp": map[string]bool{
			"configured": smtpConfigured,
		},
	})
}

// DELETE /api/admin/users/{id}
func (a *api) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	// don't allow deleting yourself from admin panel
	me, err := a.currentUser(r)
	if err == nil && me != nil && me.ID == id {
		writeError(w, 400, "cannot delete yourself")
		return
	}
	if err := a.store.DeleteUser(r.Context(), id); err != nil {
		if err == ErrNotFound {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("admin delete user", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
