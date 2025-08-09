package main

import (
    "encoding/json"
    "net/http"
    "sync"
    "time"
)

type Event struct {
    Type    string `json:"type"`
    Entity  string `json:"entity,omitempty"`
    BoardID int64  `json:"board_id"`
    ListID  *int64 `json:"list_id,omitempty"`
    Payload any    `json:"payload,omitempty"`
}

type EventBus struct {
    mu   sync.RWMutex
    subs map[int64]map[chan []byte]struct{}
}

func NewEventBus() *EventBus { return &EventBus{subs: make(map[int64]map[chan []byte]struct{})} }

func (b *EventBus) Subscribe(boardID int64) (ch chan []byte, cancel func()) {
    ch = make(chan []byte, 16)
    b.mu.Lock()
    if b.subs[boardID] == nil { b.subs[boardID] = make(map[chan []byte]struct{}) }
    b.subs[boardID][ch] = struct{}{}
    b.mu.Unlock()
    return ch, func() {
        b.mu.Lock()
        if subs, ok := b.subs[boardID]; ok {
            delete(subs, ch)
            if len(subs) == 0 { delete(b.subs, boardID) }
        }
        b.mu.Unlock()
        close(ch)
    }
}

func (b *EventBus) Publish(ev Event) {
    data, _ := json.Marshal(ev)
    b.mu.RLock()
    subs := b.subs[ev.BoardID]
    for ch := range subs {
        select { case ch <- data: default: /* drop if slow */ }
    }
    b.mu.RUnlock()
}

// Serve a single SSE connection for the given board.
func (b *EventBus) ServeSSE(w http.ResponseWriter, r *http.Request, boardID int64) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    flusher, ok := w.(http.Flusher)
    if !ok { http.Error(w, "stream unsupported", http.StatusInternalServerError); return }

    ch, cancel := b.Subscribe(boardID)
    defer cancel()

    // Initial comment to open the stream
    _, _ = w.Write([]byte(": connected\n\n"))
    flusher.Flush()

    ticker := time.NewTicker(25 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-r.Context().Done():
            return
        case <-ticker.C:
            // heartbeat comment to keep connection alive through proxies
            _, _ = w.Write([]byte(": ping\n\n"))
            flusher.Flush()
        case msg, ok := <-ch:
            if !ok { return }
            _, _ = w.Write([]byte("data: "))
            _, _ = w.Write(msg)
            _, _ = w.Write([]byte("\n\n"))
            flusher.Flush()
        }
    }
}
