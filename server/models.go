package main

import "time"

type Board struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Color string `json:"color,omitempty"`
	// Pos is used for ordering boards
	// omitted from JSON to keep payloads small; ordering happens server-side
	CreatedAt time.Time `json:"created_at"`
	ProjectID *int64    `json:"project_id,omitempty"`
	CreatedBy *int64    `json:"created_by,omitempty"`
	// ViaGroup indicates the board is accessible to the current user via their group membership
	ViaGroup bool `json:"via_group,omitempty"`
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
	ID              int64      `json:"id"`
	ListID          int64      `json:"list_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	DescriptionIsMD bool       `json:"description_is_md"`
	Color           string     `json:"color,omitempty"`
	Pos             int64      `json:"pos"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
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
	// Role is the current user's role in this group when applicable (1=member, 2=admin)
	Role int `json:"role,omitempty"`
}
