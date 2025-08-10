package main

import (
	"net/http"
	"strings"
)

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
