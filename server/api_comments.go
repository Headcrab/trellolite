package main

import "net/http"

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
