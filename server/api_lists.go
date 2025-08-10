package main

import (
	"errors"
	"net/http"
)

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
		a.bus.Publish(Event{Type: "list.moved", Entity: "list", BoardID: dstBid, ListID: &id, Payload: map[string]any{"id": id, "new_index": req.NewIndex}})
		if srcBid != 0 && srcBid != dstBid {
			a.bus.Publish(Event{Type: "list.deleted", Entity: "list", BoardID: srcBid, ListID: &id, Payload: map[string]any{"id": id}})
		}
	}
}
