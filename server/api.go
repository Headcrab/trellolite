package main

import (
	"net/http"
	"time"
)

// core helpers moved to api_core.go

// Auth & OAuth helpers moved to api_auth.go

func (a *api) routes(mux *http.ServeMux) {
	// Auth endpoints
	mux.HandleFunc("POST /api/auth/register", a.withRateLimit("auth", 20, time.Minute, a.handleRegister))
	mux.HandleFunc("POST /api/auth/login", a.withRateLimit("auth", 30, time.Minute, a.handleLogin))
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)
	mux.HandleFunc("GET /api/auth/providers", a.handleAuthProviders)
	mux.HandleFunc("GET /api/auth/oauth/github/start", a.handleGithubStart)
	mux.HandleFunc("GET /api/auth/oauth/github/callback", a.handleGithubCallback)
	mux.HandleFunc("GET /api/auth/oauth/google/start", a.handleGoogleStart)
	mux.HandleFunc("GET /api/auth/oauth/google/callback", a.handleGoogleCallback)

	// Profile / self-update
	mux.HandleFunc("PATCH /api/me", a.requireAuth(a.handleUpdateMe))

	// Dev password reset (magic link in logs)
	mux.HandleFunc("POST /api/auth/reset", a.withRateLimit("auth_reset", 10, time.Minute, a.handleResetRequest))
	mux.HandleFunc("POST /api/auth/reset/confirm", a.withRateLimit("auth_reset", 20, time.Minute, a.handleResetConfirm))
	mux.HandleFunc("POST /api/auth/verify/confirm", a.withRateLimit("auth_verify", 20, time.Minute, a.handleVerifyConfirm))

	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/boards", a.handleListBoards)
	mux.HandleFunc("POST /api/boards", a.requireAuth(a.handleCreateBoard))
	mux.HandleFunc("GET /api/boards/{id}", a.requireAuth(a.handleGetBoard))
	mux.HandleFunc("GET /api/boards/{id}/full", a.requireAuth(a.handleGetBoardFull))
	mux.HandleFunc("GET /api/boards/{id}/events", a.requireAuth(a.handleBoardEvents))
	mux.HandleFunc("GET /api/boards/{id}/members", a.requireAuth(a.handleBoardMembers))
	mux.HandleFunc("PATCH /api/boards/{id}", a.requireAuth(a.handleUpdateBoard))
	mux.HandleFunc("POST /api/boards/{id}/move", a.requireAuth(a.handleMoveBoard))
	mux.HandleFunc("DELETE /api/boards/{id}", a.requireAuth(a.handleDeleteBoard))

	mux.HandleFunc("GET /api/boards/{id}/lists", a.requireAuth(a.handleListsByBoard))
	mux.HandleFunc("POST /api/boards/{id}/lists", a.requireAuth(a.handleCreateList))
	mux.HandleFunc("PATCH /api/lists/{id}", a.requireAuth(a.handleUpdateList))
	mux.HandleFunc("POST /api/lists/{id}/move", a.requireAuth(a.handleMoveList))
	mux.HandleFunc("DELETE /api/lists/{id}", a.requireAuth(a.handleDeleteList))

	mux.HandleFunc("GET /api/lists/{id}/cards", a.requireAuth(a.handleCardsByList))
	mux.HandleFunc("POST /api/lists/{id}/cards", a.requireAuth(a.handleCreateCard))
	mux.HandleFunc("PATCH /api/cards/{id}", a.requireAuth(a.handleUpdateCard))
	mux.HandleFunc("DELETE /api/cards/{id}", a.requireAuth(a.handleDeleteCard))
	mux.HandleFunc("POST /api/cards/{id}/move", a.requireAuth(a.handleMoveCard))

	mux.HandleFunc("GET /api/cards/{id}/comments", a.requireAuth(a.handleCommentsByCard))
	mux.HandleFunc("POST /api/cards/{id}/comments", a.requireAuth(a.handleAddComment))

	// Groups and board visibility
	mux.HandleFunc("POST /api/groups", a.requireAuth(a.handleCreateGroupSelf))
	mux.HandleFunc("GET /api/my/groups", a.requireAuth(a.handleMyGroups))
	// Self-managed groups (creators/admins)
	mux.HandleFunc("GET /api/groups/{id}/users", a.requireAuth(a.handleSelfGroupUsers))
	mux.HandleFunc("GET /api/groups/{id}/users/search", a.requireAuth(a.handleSelfSearchUsers))
	mux.HandleFunc("POST /api/groups/{id}/users", a.requireAuth(a.handleSelfAddUserToGroup))
	mux.HandleFunc("DELETE /api/groups/{id}/users/{uid}", a.requireAuth(a.handleSelfRemoveUserFromGroup))
	// Self leave from a group (any member can leave)
	mux.HandleFunc("POST /api/groups/{id}/leave", a.requireAuth(a.handleSelfLeaveGroup))
	mux.HandleFunc("DELETE /api/groups/{id}", a.requireAuth(a.handleSelfDeleteGroup))
	mux.HandleFunc("GET /api/boards/{id}/groups", a.requireAuth(a.handleBoardGroups))
	mux.HandleFunc("POST /api/boards/{id}/groups", a.requireAuth(a.handleBoardGroupAdd))
	mux.HandleFunc("DELETE /api/boards/{id}/groups/{gid}", a.requireAuth(a.handleBoardGroupRemove))

	// Admin: groups CRUD and membership
	mux.HandleFunc("GET /api/admin/groups", a.requireAdmin(a.handleAdminListGroups))
	mux.HandleFunc("POST /api/admin/groups", a.requireAdmin(a.handleAdminCreateGroup))
	mux.HandleFunc("DELETE /api/admin/groups/{id}", a.requireAdmin(a.handleAdminDeleteGroup))
	mux.HandleFunc("GET /api/admin/groups/{id}/users", a.requireAdmin(a.handleAdminGroupUsers))
	mux.HandleFunc("POST /api/admin/groups/{id}/users", a.requireAdmin(a.handleAdminAddUserToGroup))
	mux.HandleFunc("DELETE /api/admin/groups/{id}/users/{uid}", a.requireAdmin(a.handleAdminRemoveUserFromGroup))
	mux.HandleFunc("GET /api/admin/users", a.requireAdmin(a.handleAdminListUsers))
	mux.HandleFunc("PATCH /api/admin/users/{id}", a.requireAdmin(a.handleAdminUpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{id}", a.requireAdmin(a.handleAdminDeleteUser))
	mux.HandleFunc("GET /api/admin/system", a.requireAdmin(a.handleAdminSystemStatus))

	// Projects
	mux.HandleFunc("GET /api/projects", a.requireAuth(a.handleListProjects))
	mux.HandleFunc("POST /api/projects", a.requireAuth(a.handleCreateProject))
	mux.HandleFunc("GET /api/projects/{id}/members", a.requireAuth(a.handleProjectMembers))
	mux.HandleFunc("POST /api/projects/{id}/members", a.requireAuth(a.handleAddProjectMember))
	mux.HandleFunc("DELETE /api/projects/{id}/members/{uid}", a.requireAuth(a.handleRemoveProjectMember))
}

// Handlers implementation moved into separate files under server/:
//  - api_health.go, api_auth.go, api_boards.go, api_lists.go, api_cards.go,
//    api_comments.go, api_groups.go, api_admin.go, api_projects.go
