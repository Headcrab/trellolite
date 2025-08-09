package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type api struct {
	store *Store
	log   *slog.Logger
	bus   *EventBus
}

func newAPI(store *Store, log *slog.Logger) *api {
	return &api{store: store, log: log, bus: NewEventBus()}
}

func parseID(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, r.Body)
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": msg})
}

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/boards", a.handleListBoards)
	mux.HandleFunc("POST /api/boards", a.handleCreateBoard)
	mux.HandleFunc("GET /api/boards/{id}", a.handleGetBoard)
	mux.HandleFunc("GET /api/boards/{id}/full", a.handleGetBoardFull)
	mux.HandleFunc("GET /api/boards/{id}/events", a.handleBoardEvents)
	mux.HandleFunc("PATCH /api/boards/{id}", a.handleUpdateBoard)
	mux.HandleFunc("POST /api/boards/{id}/move", a.handleMoveBoard)
	mux.HandleFunc("DELETE /api/boards/{id}", a.handleDeleteBoard)

	mux.HandleFunc("GET /api/boards/{id}/lists", a.handleListsByBoard)
	mux.HandleFunc("POST /api/boards/{id}/lists", a.handleCreateList)
	mux.HandleFunc("PATCH /api/lists/{id}", a.handleUpdateList)
	mux.HandleFunc("POST /api/lists/{id}/move", a.handleMoveList)
	mux.HandleFunc("DELETE /api/lists/{id}", a.handleDeleteList)

	mux.HandleFunc("GET /api/lists/{id}/cards", a.handleCardsByList)
	mux.HandleFunc("POST /api/lists/{id}/cards", a.handleCreateCard)
	mux.HandleFunc("PATCH /api/cards/{id}", a.handleUpdateCard)
	mux.HandleFunc("DELETE /api/cards/{id}", a.handleDeleteCard)
	mux.HandleFunc("POST /api/cards/{id}/move", a.handleMoveCard)

	mux.HandleFunc("GET /api/cards/{id}/comments", a.handleCommentsByCard)
	mux.HandleFunc("POST /api/cards/{id}/comments", a.handleAddComment)
}

func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "ts": time.Now().UTC().Format(time.RFC3339)})
}

func (a *api) handleListBoards(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListBoards(r.Context())
	if err != nil {
		a.log.Error("list boards", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}
func (a *api) handleCreateBoard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := readJSON(w, r, &req); err != nil || len(req.Title) == 0 {
		if err != nil {
			a.log.Error("decode create board", "err", err)
		}
		writeError(w, 400, "invalid payload")
		return
	}
	b, err := a.store.CreateBoard(r.Context(), req.Title)
	if err != nil {
		a.log.Error("create board", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, b)
}
func (a *api) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
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
	// publish update event
	a.bus.Publish(Event{Type: "board.updated", Entity: "board", BoardID: id, Payload: map[string]any{"id": id}})
}
func (a *api) handleDeleteBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
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

func (a *api) handleListsByBoard(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
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
	// publish list created
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
	// detect source board to emit events
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
		// notify destination board
		a.bus.Publish(Event{Type: "list.moved", Entity: "list", BoardID: dstBid, ListID: &id, Payload: map[string]any{"id": id, "new_index": req.NewIndex}})
		// if moved across boards, notify source board about deletion of this list
		if srcBid != 0 && srcBid != dstBid {
			a.bus.Publish(Event{Type: "list.deleted", Entity: "list", BoardID: srcBid, ListID: &id, Payload: map[string]any{"id": id}})
		}
	}
}

func (a *api) handleCardsByList(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
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
	var req struct{ Title, Description string }
	if err := readJSON(w, r, &req); err != nil || len(req.Title) == 0 {
		if err != nil {
			a.log.Error("decode create card", "err", err)
		}
		writeError(w, 400, "invalid payload")
		return
	}
	c, err := a.store.CreateCard(r.Context(), id, req.Title, req.Description)
	if err != nil {
		a.log.Error("create card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 201, c)
	// Publish on card create
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
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Pos         *int64  `json:"pos"`
		DueAt       *string `json:"due_at"`
		Color       *string `json:"color"`
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
	if err := a.store.UpdateCard(r.Context(), id, req.Title, req.Description, req.Pos, due); err != nil {
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
		a.bus.Publish(Event{Type: "card.updated", Entity: "card", BoardID: bid, Payload: map[string]any{"id": id}})
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
	items, err := a.store.CommentsByCard(r.Context(), id)
	if err != nil {
		a.log.Error("comments by card", "err", err)
		writeError(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, items)
}

// SSE endpoint for a board
func (a *api) handleBoardEvents(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
	}
	a.bus.ServeSSE(w, r, id)
}

// Aggregate endpoint: board with lists and cards
func (a *api) handleGetBoardFull(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "bad id")
		return
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

func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(sw, r)
		log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur_ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

// Implement http.Flusher if underlying writer supports it (needed for SSE)
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
