package main

import (
	"net/http"
	"strconv"
	"strings"
)

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

// Self-managed groups
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
