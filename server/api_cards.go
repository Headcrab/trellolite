package main

import (
	"database/sql"
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
		ParentID        *int64 `json:"parent_id"`
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
	// apply parent if provided
	if req.ParentID != nil {
		_ = a.store.UpdateCard(r.Context(), c.ID, nil, nil, nil, nil, nil, nil, req.ParentID)
		c.ParentID = req.ParentID
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
		ParentID        *int64  `json:"parent_id"`
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
	// normalize parent_id: 0 -> NULL
	if req.ParentID != nil && *req.ParentID == 0 {
		req.ParentID = nil
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

	// If parent change requested, validate and ensure list alignment
	if req.ParentID != nil {
		if *req.ParentID == id {
			writeError(w, 400, "cannot set parent to self")
			return
		}
		// Ensure parent exists and not descendant of id
		var parentList int64
		if err := a.store.db.QueryRowContext(r.Context(), `select list_id from cards where id=$1`, *req.ParentID).Scan(&parentList); err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
				writeError(w, 404, "parent not found")
				return
			}
		}
		// Prevent cycles: check if target parent is a descendant of current card
		var isDesc bool
		q := `with recursive sub(id) as (select id from cards where parent_card_id=$1 union all select c.id from cards c join sub s on c.parent_card_id=s.id) select exists(select 1 from sub where id=$2)`
		if err := a.store.db.QueryRowContext(r.Context(), q, id, *req.ParentID).Scan(&isDesc); err == nil && isDesc {
			writeError(w, 400, "cannot set parent to descendant")
			return
		}
		// Align list with parent list if differs (move subtree)
		if _, curList, e := a.store.BoardAndListByCard(r.Context(), id); e == nil && curList != parentList {
			_ = a.store.MoveCard(r.Context(), id, parentList, 1<<30)
		}
	}

	if err := a.store.UpdateCard(r.Context(), id, req.Title, req.Description, req.Pos, due, req.DescriptionIsMD, req.AssigneeID, req.ParentID); err != nil {
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

// handleCreateOrGetShare creates or returns existing share token for a card the user can access.
// Response: { token: string, url: string }
func (a *api) handleCreateOrGetShare(w http.ResponseWriter, r *http.Request) {
	cardID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	u, errU := a.currentUser(r)
	if errU != nil {
		writeError(w, 401, "unauthorized")
		return
	}
	ok, err := a.store.CanAccessCard(r.Context(), u.ID, cardID)
	if err != nil {
		a.log.Error("share access check", "err", err)
	}
	if !ok {
		writeError(w, 403, "forbidden")
		return
	}
	token, err := a.store.GetOrCreateCardShare(r.Context(), cardID)
	if err != nil {
		a.log.Error("create share token", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	// derive base URL
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// honor X-Forwarded-Proto if present (behind proxy)
	if xf := r.Header.Get("X-Forwarded-Proto"); xf == "https" || xf == "http" {
		scheme = xf
	}
	url := scheme + "://" + r.Host + "/share/" + token
	writeJSON(w, 200, map[string]any{"token": token, "url": url})
}

// handlePublicSharePage serves the public share HTML page. No auth required.
func (a *api) handlePublicSharePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./web/share.html")
}

// handlePublicShareData returns public JSON for a shared card by token. No auth required.
func (a *api) handlePublicShareData(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		writeError(w, 400, "bad token")
		return
	}
	c, err := a.store.CardByShareToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, 404, "not found")
			return
		}
		a.log.Error("share fetch", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	// Include comments for this card in the public payload
	comments, err2 := a.store.CommentsByCard(r.Context(), c.ID)
	if err2 != nil {
		// Log but don't fail the whole request
		a.log.Error("share comments fetch", "err", err2)
		comments = nil
	}
	writeJSON(w, 200, map[string]any{
		"card":     c,
		"comments": comments,
	})
}
