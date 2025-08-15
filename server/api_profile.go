package main

import (
	"net/http"
	"strings"
)

// PATCH /api/me { name }
// Updates current user's basic profile fields (currently only name)
func (a *api) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	me, err := a.currentUser(r)
	if err != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	var req struct {
		Name *string `json:"name"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	if req.Name == nil {
		writeError(w, 400, "name required")
		return
	}
	v := strings.TrimSpace(*req.Name)
	if v == "" {
		writeError(w, 400, "name required")
		return
	}
	if err := a.store.AdminUpdateUser(r.Context(), me.ID, &v, nil, nil, nil, nil); err != nil {
		a.log.Error("update me", "err", err)
		writeError(w, 400, "cannot update profile")
		return
	}
	// reload fresh user
	u, err := a.currentUser(r)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "user": u})
}
