package main

import (
	"errors"
	"net/http"
	"time"
)

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
	var req struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		DescriptionIsMD bool   `json:"description_is_md"`
	}
	if err := readJSON(w, r, &req); err != nil || len(req.Title) == 0 {
		if err != nil {
			a.log.Error("decode create card", "err", err)
		}
		writeError(w, 400, "invalid payload")
		return
	}
	c, err := a.store.CreateCard(r.Context(), id, req.Title, req.Description, req.DescriptionIsMD)
	if err != nil {
		a.log.Error("create card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, c)
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
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		Pos             *int64  `json:"pos"`
		DueAt           *string `json:"due_at"`
		Color           *string `json:"color"`
		DescriptionIsMD *bool   `json:"description_is_md"`
		AssigneeID      *int64  `json:"assignee_id"`
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
	// Validate assignee belongs to the same board members set (if provided)
	if req.AssigneeID != nil {
		if *req.AssigneeID == 0 {
			// treat 0 as NULL to clear assignee
			req.AssigneeID = nil
		} else {
			// ensure the user is among board members
			if bid, _, e := a.store.BoardAndListByCard(r.Context(), id); e == nil {
				members, e2 := a.store.BoardMembers(r.Context(), bid)
				if e2 == nil {
					found := false
					for _, m := range members {
						if m.ID == *req.AssigneeID {
							found = true
							break
						}
					}
					if !found {
						writeError(w, 400, "assignee must be a board member")
						return
					}
				}
			}
		}
	}

	if err := a.store.UpdateCard(r.Context(), id, req.Title, req.Description, req.Pos, due, req.DescriptionIsMD, req.AssigneeID); err != nil {
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
		if req.AssigneeID != nil {
			a.bus.Publish(Event{Type: "card.assignee_changed", Entity: "card", BoardID: bid, Payload: map[string]any{"id": id, "assignee_id": req.AssigneeID}})
		} else {
			a.bus.Publish(Event{Type: "card.updated", Entity: "card", BoardID: bid, Payload: map[string]any{"id": id}})
		}
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
