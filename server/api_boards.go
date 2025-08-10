package main

import (
	"errors"
	"net/http"
	"strings"
)

func (a *api) handleListBoards(w http.ResponseWriter, r *http.Request) {
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
		if ok, _ := a.store.CanAccessProject(r.Context(), req.ProjectID, u.ID); ok {
			_ = a.store.SetBoardProject(r.Context(), b.ID, req.ProjectID)
		}
	}
	writeJSON(w, 201, b)
}

func (a *api) handleGetBoard(w http.ResponseWriter, r *http.Request) {
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

func (a *api) handleGetBoardFull(w http.ResponseWriter, r *http.Request) {
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
