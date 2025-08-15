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
		Lang *string `json:"lang"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, 400, "invalid payload")
		return
	}
	var nameVal *string
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" {
			writeError(w, 400, "name required")
			return
		}
		nameVal = &v
	}
	// sanitize lang
	var langVal *string
	if req.Lang != nil {
		v := strings.TrimSpace(*req.Lang)
		// allow only en|ru|auto
		switch strings.ToLower(v) {
		case "en", "ru", "auto", "":
			// ok
		default:
			v = "auto"
		}
		langVal = &v
	}
	if nameVal == nil && langVal == nil {
		writeError(w, 400, "nothing to update")
		return
	}
	if err := a.store.AdminUpdateUser(r.Context(), me.ID, nameVal, nil, nil, nil, nil, langVal); err != nil {
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
