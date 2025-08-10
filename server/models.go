package main

import "time"

type Board struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Color string `json:"color,omitempty"`
	// Pos is used for ordering boards
	// omitted from JSON to keep payloads small; ordering happens server-side
	CreatedAt time.Time `json:"created_at"`
}

type List struct {
	ID        int64     `json:"id"`
	BoardID   int64     `json:"board_id"`
	Title     string    `json:"title"`
	Color     string    `json:"color,omitempty"`
	Pos       int64     `json:"pos"`
	CreatedAt time.Time `json:"created_at"`
}

type Card struct {
	ID          int64      `json:"id"`
	ListID      int64      `json:"list_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Color       string     `json:"color,omitempty"`
	Pos         int64      `json:"pos"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Comment struct {
	ID        int64     `json:"id"`
	CardID    int64     `json:"card_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Below are preliminary models for upcoming auth/admin features.
// They are not yet wired into the API and exist to maintain type discipline.

type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	IsActive  bool      `json:"is_active"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type Project struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	OwnerID   int64     `json:"owner_user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Group struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
