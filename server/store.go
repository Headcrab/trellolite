package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) ListBoards(ctx context.Context, userID int64, scope string) ([]Board, error) {
	var rows *sql.Rows
	var err error
	switch strings.ToLower(scope) {
	case "groups", "group":
		rows, err = s.db.QueryContext(ctx, `
			select b.id, b.title, coalesce(b.color,''), b.created_at, b.project_id, b.created_by,
				   true as via_group
			from boards b
			where exists (
				select 1 from board_groups bg
				join user_groups ug on ug.group_id = bg.group_id
				where bg.board_id = b.id and ug.user_id = $1
			)
			order by b.pos, b.id`, userID)
	case "all":
		rows, err = s.db.QueryContext(ctx, `
			select b.id, b.title, coalesce(b.color,''), b.created_at, b.project_id, b.created_by,
				   exists (
					   select 1 from board_groups bg
					   join user_groups ug on ug.group_id = bg.group_id
					   where bg.board_id = b.id and ug.user_id = $1
				   ) as via_group
			from boards b
			where b.created_by = $1
			   or exists (
				   select 1 from board_groups bg
				   join user_groups ug on ug.group_id = bg.group_id
				   where bg.board_id = b.id and ug.user_id = $1
			   )
			   or exists (
				   select 1 from projects p left join project_members pm on pm.project_id = p.id and pm.user_id = $1
				   where p.id = b.project_id and (p.owner_user_id = $1 or pm.user_id is not null)
			   )
			order by b.pos, b.id`, userID)
	default: // "mine"
		rows, err = s.db.QueryContext(ctx, `
			select id, title, coalesce(color,''), created_at, project_id, created_by,
				   false as via_group
			from boards
			where created_by = $1
			order by pos, id`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.Title, &b.Color, &b.CreatedAt, &b.ProjectID, &b.CreatedBy, &b.ViaGroup); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) CreateBoard(ctx context.Context, userID int64, title string) (Board, error) {
	var next int64 = 1000
	_ = s.db.QueryRowContext(ctx, `select coalesce(max(pos),0)+1000 from boards`).Scan(&next)
	var b Board
	err := s.db.QueryRowContext(ctx, `insert into boards(title, pos, created_by) values($1,$2,$3) returning id, title, coalesce(color,''), created_at, created_by`, title, next, userID).
		Scan(&b.ID, &b.Title, &b.Color, &b.CreatedAt, &b.CreatedBy)
	return b, err
}

// Projects
func (s *Store) ListProjects(ctx context.Context, userID int64) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		select p.id, p.name, p.owner_user_id, p.created_at
		from projects p
		left join project_members pm on pm.project_id = p.id and pm.user_id = $1
		where p.owner_user_id = $1 or pm.user_id is not null
		order by p.created_at desc, p.id desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.OwnerID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateProject(ctx context.Context, ownerID int64, name string) (Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx, `insert into projects(name, owner_user_id) values($1,$2) returning id, name, owner_user_id, created_at`, name, ownerID).
		Scan(&p.ID, &p.Name, &p.OwnerID, &p.CreatedAt)
	return p, err
}

func (s *Store) AddProjectMember(ctx context.Context, projectID, userID int64, role int) error {
	if role <= 0 {
		role = 2
	}
	_, err := s.db.ExecContext(ctx, `insert into project_members(project_id, user_id, role) values($1,$2,$3)
		on conflict (project_id, user_id) do update set role=excluded.role`, projectID, userID, role)
	return err
}

func (s *Store) RemoveProjectMember(ctx context.Context, projectID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `delete from project_members where project_id=$1 and user_id=$2`, projectID, userID)
	return err
}

func (s *Store) ProjectMembers(ctx context.Context, projectID int64) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `select u.id, u.email, u.name, coalesce(u.avatar_url,''), u.is_active, u.is_admin, coalesce(u.email_verified,false), u.created_at
		from project_members pm join users u on u.id = pm.user_id where pm.project_id=$1 order by u.email`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.EmailVerified, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// BoardMembers returns distinct users who have access/participate in the board:
// - board owner
// - users from groups that have access to the board
// - project owner and project members, if the board is linked to a project
func (s *Store) BoardMembers(ctx context.Context, boardID int64) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		with owners as (
			select u.id, u.email, u.name, coalesce(u.avatar_url,'') as avatar_url, u.is_active, u.is_admin, coalesce(u.email_verified,false) as email_verified, u.created_at
			from boards b join users u on u.id = b.created_by
			where b.id = $1
		),
		via_groups as (
			select u.id, u.email, u.name, coalesce(u.avatar_url,'') as avatar_url, u.is_active, u.is_admin, coalesce(u.email_verified,false) as email_verified, u.created_at
			from board_groups bg
			join user_groups ug on ug.group_id = bg.group_id
			join users u on u.id = ug.user_id
			where bg.board_id = $1
		),
		project_side as (
			select u.id, u.email, u.name, coalesce(u.avatar_url,'') as avatar_url, u.is_active, u.is_admin, coalesce(u.email_verified,false) as email_verified, u.created_at
			from boards b
			join projects p on p.id = b.project_id
			join users u on u.id = p.owner_user_id
			where b.id = $1 and b.project_id is not null
			union
			select u.id, u.email, u.name, coalesce(u.avatar_url,'') as avatar_url, u.is_active, u.is_admin, coalesce(u.email_verified,false) as email_verified, u.created_at
			from boards b
			join project_members pm on pm.project_id = b.project_id
			join users u on u.id = pm.user_id
			where b.id = $1 and b.project_id is not null
		)
		select distinct id, email, name, avatar_url, is_active, is_admin, email_verified, created_at
		from (
			select * from owners
			union all
			select * from via_groups
			union all
			select * from project_side
		) allu
		order by email
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.EmailVerified, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) SetBoardProject(ctx context.Context, boardID, projectID int64) error {
	_, err := s.db.ExecContext(ctx, `update boards set project_id=$1 where id=$2`, projectID, boardID)
	return err
}

func (s *Store) IsProjectOwner(ctx context.Context, projectID, userID int64) (bool, error) {
	var x int
	err := s.db.QueryRowContext(ctx, `select 1 from projects where id=$1 and owner_user_id=$2`, projectID, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CanAccessProject(ctx context.Context, projectID, userID int64) (bool, error) {
	var x int
	if err := s.db.QueryRowContext(ctx, `select 1 from projects where id=$1 and owner_user_id=$2`, projectID, userID).Scan(&x); err == nil {
		return true, nil
	}
	err := s.db.QueryRowContext(ctx, `select 1 from project_members where project_id=$1 and user_id=$2`, projectID, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) GetBoard(ctx context.Context, id int64) (Board, error) {
	var b Board
	err := s.db.QueryRowContext(ctx, `select id, title, coalesce(color,''), created_at, project_id, created_by from boards where id=$1`, id).
		Scan(&b.ID, &b.Title, &b.Color, &b.CreatedAt, &b.ProjectID, &b.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Board{}, ErrNotFound
	}
	return b, err
}

func (s *Store) UpdateBoard(ctx context.Context, id int64, title string) error {
	res, err := s.db.ExecContext(ctx, `update boards set title=$1 where id=$2`, title, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteBoard(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `delete from boards where id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListsByBoard(ctx context.Context, boardID int64) ([]List, error) {
	rows, err := s.db.QueryContext(ctx,
		`select id, board_id, title, coalesce(color,''), pos, created_at from lists where board_id=$1 order by pos, id`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []List
	for rows.Next() {
		var l List
		if err := rows.Scan(&l.ID, &l.BoardID, &l.Title, &l.Color, &l.Pos, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) CreateList(ctx context.Context, boardID int64, title string) (List, error) {
	var next int64 = 1000
	_ = s.db.QueryRowContext(ctx, `select coalesce(max(pos),0)+1000 from lists where board_id=$1`, boardID).Scan(&next)
	var l List
	err := s.db.QueryRowContext(ctx,
		`insert into lists(board_id, title, pos) values($1,$2,$3) returning id, board_id, title, coalesce(color,''), pos, created_at`,
		boardID, title, next).
		Scan(&l.ID, &l.BoardID, &l.Title, &l.Color, &l.Pos, &l.CreatedAt)
	return l, err
}

func (s *Store) UpdateList(ctx context.Context, id int64, title *string, pos *int64) error {
	if title == nil && pos == nil {
		return nil
	}
	if title != nil && pos != nil {
		_, err := s.db.ExecContext(ctx, `update lists set title=$1, pos=$2 where id=$3`, *title, *pos, id)
		if err != nil {
			return err
		}
	} else if title != nil {
		_, err := s.db.ExecContext(ctx, `update lists set title=$1 where id=$2`, *title, id)
		if err != nil {
			return err
		}
	} else {
		_, err := s.db.ExecContext(ctx, `update lists set pos=$1 where id=$2`, *pos, id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetList(ctx context.Context, id int64) (List, error) {
	var l List
	err := s.db.QueryRowContext(ctx, `select id, board_id, title, coalesce(color,''), pos, created_at from lists where id=$1`, id).
		Scan(&l.ID, &l.BoardID, &l.Title, &l.Color, &l.Pos, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return List{}, ErrNotFound
	}
	return l, err
}

func (s *Store) DeleteList(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `delete from lists where id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CardsByList(ctx context.Context, listID int64) ([]Card, error) {
	rows, err := s.db.QueryContext(ctx,
		`select id, list_id, title, description, coalesce(color,''), pos, due_at, assignee_user_id, created_at, coalesce(description_is_md,false)
	 from cards where list_id=$1 order by pos, id`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.ListID, &c.Title, &c.Description, &c.Color, &c.Pos, &c.DueAt, &c.AssigneeUserID, &c.CreatedAt, &c.DescriptionIsMD); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateCard(ctx context.Context, listID int64, title, description string, isMD bool) (Card, error) {
	var next int64 = 1000
	_ = s.db.QueryRowContext(ctx, `select coalesce(max(pos),0)+1000 from cards where list_id=$1`, listID).Scan(&next)
	var c Card
	err := s.db.QueryRowContext(ctx,
		`insert into cards(list_id, title, description, pos, description_is_md) values($1,$2,$3,$4,$5)
	 returning id, list_id, title, description, coalesce(color,''), pos, due_at, assignee_user_id, created_at, coalesce(description_is_md,false)`,
		listID, title, description, next, isMD).
		Scan(&c.ID, &c.ListID, &c.Title, &c.Description, &c.Color, &c.Pos, &c.DueAt, &c.AssigneeUserID, &c.CreatedAt, &c.DescriptionIsMD)
	return c, err
}

// --- Groups & board visibility ---
func (s *Store) MyGroups(ctx context.Context, userID int64) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `select g.id, g.name, g.created_at, ug.role
		from groups g join user_groups ug on ug.group_id = g.id
		where ug.user_id=$1 order by g.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.Role); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Self-managed group helpers
// IsGroupAdmin returns true if user has role>=2 in the group
func (s *Store) IsGroupAdmin(ctx context.Context, groupID, userID int64) (bool, error) {
	var x int
	err := s.db.QueryRowContext(ctx, `select 1 from user_groups where group_id=$1 and user_id=$2 and role>=2`, groupID, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) GroupUsersForSelf(ctx context.Context, groupID int64) ([]User, error) {
	return s.GroupUsers(ctx, groupID)
}

func (s *Store) AddUserToGroupSelf(ctx context.Context, groupID, userID int64) error {
	return s.AddUserToGroup(ctx, groupID, userID)
}

func (s *Store) RemoveUserFromGroupSelf(ctx context.Context, groupID, userID int64) error {
	return s.RemoveUserFromGroup(ctx, groupID, userID)
}

func (s *Store) DeleteGroupIfAdmin(ctx context.Context, groupID, userID int64) error {
	ok, err := s.IsGroupAdmin(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("forbidden")
	}
	return s.DeleteGroup(ctx, groupID)
}

func (s *Store) BoardGroups(ctx context.Context, boardID int64) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `select g.id, g.name, g.created_at
		from board_groups bg join groups g on g.id = bg.group_id
		where bg.board_id=$1 order by g.name`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) AddBoardToGroup(ctx context.Context, boardID, groupID int64) error {
	_, err := s.db.ExecContext(ctx, `insert into board_groups(board_id, group_id) values($1,$2) on conflict do nothing`, boardID, groupID)
	return err
}

func (s *Store) RemoveBoardFromGroup(ctx context.Context, boardID, groupID int64) error {
	_, err := s.db.ExecContext(ctx, `delete from board_groups where board_id=$1 and group_id=$2`, boardID, groupID)
	return err
}

func (s *Store) IsBoardOwner(ctx context.Context, boardID, userID int64) (bool, error) {
	var x int
	err := s.db.QueryRowContext(ctx, `select 1 from boards where id=$1 and created_by=$2`, boardID, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) IsUserInGroup(ctx context.Context, userID, groupID int64) (bool, error) {
	var x int
	err := s.db.QueryRowContext(ctx, `select 1 from user_groups where user_id=$1 and group_id=$2`, userID, groupID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// --- Groups CRUD & membership (admin) ---
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `select id, name, created_at from groups order by name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateGroup(ctx context.Context, name string) (Group, error) {
	var g Group
	err := s.db.QueryRowContext(ctx, `insert into groups(name) values($1) returning id, name, created_at`, name).Scan(&g.ID, &g.Name, &g.CreatedAt)
	return g, err
}

// CreateGroupOwned creates a group and adds the owner as admin (role=2)
func (s *Store) CreateGroupOwned(ctx context.Context, ownerID int64, name string) (Group, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Group{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var g Group
	if err := tx.QueryRowContext(ctx, `insert into groups(name) values($1) returning id, name, created_at`, name).Scan(&g.ID, &g.Name, &g.CreatedAt); err != nil {
		return Group{}, err
	}
	// role: 2 = admin, 1 = member
	if _, err := tx.ExecContext(ctx, `insert into user_groups(user_id, group_id, role) values($1,$2,2) on conflict (user_id, group_id) do update set role=excluded.role`, ownerID, g.ID); err != nil {
		return Group{}, err
	}
	if err := tx.Commit(); err != nil {
		return Group{}, err
	}
	return g, nil
}

func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `delete from groups where id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GroupUsers(ctx context.Context, groupID int64) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `select u.id, u.email, u.name, coalesce(u.avatar_url,''), u.is_active, u.is_admin, coalesce(u.email_verified,false), u.created_at
			from user_groups ug join users u on u.id = ug.user_id where ug.group_id=$1 order by u.email`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.EmailVerified, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) AddUserToGroup(ctx context.Context, groupID, userID int64) error {
	// default role=1 (member) when not specified
	_, err := s.db.ExecContext(ctx, `insert into user_groups(user_id, group_id, role) values($1,$2,1)
		on conflict (user_id, group_id) do update set role=coalesce(user_groups.role,1)`, userID, groupID)
	return err
}

func (s *Store) RemoveUserFromGroup(ctx context.Context, groupID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `delete from user_groups where user_id=$1 and group_id=$2`, userID, groupID)
	return err
}

// ListUsers returns users filtered by query (ILIKE on email or name). Limit is capped to [1,200].
func (s *Store) ListUsers(ctx context.Context, query string, limit int) ([]User, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var rows *sql.Rows
	var err error
	q := strings.TrimSpace(query)
	if q == "" {
		rows, err = s.db.QueryContext(ctx, `select id, email, name, coalesce(avatar_url,''), is_active, is_admin, coalesce(email_verified,false), created_at from users order by id desc limit $1`, limit)
	} else {
		like := "%" + q + "%"
		rows, err = s.db.QueryContext(ctx, `select id, email, name, coalesce(avatar_url,''), is_active, is_admin, coalesce(email_verified,false), created_at from users where email ilike $1 or name ilike $1 order by email limit $2`, like, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.EmailVerified, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CanAccessBoard: user is owner OR belongs to a group that has access to the board
func (s *Store) CanAccessBoard(ctx context.Context, userID, boardID int64) (bool, error) {
	var x int
	// owner
	if err := s.db.QueryRowContext(ctx, `select 1 from boards where id=$1 and created_by=$2`, boardID, userID).Scan(&x); err == nil {
		return true, nil
	}
	// via groups
	err := s.db.QueryRowContext(ctx, `select 1 from board_groups bg join user_groups ug on ug.group_id=bg.group_id where bg.board_id=$1 and ug.user_id=$2`, boardID, userID).Scan(&x)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	// via project membership
	err = s.db.QueryRowContext(ctx, `
		select 1 from boards b
		left join projects p on p.id = b.project_id
		left join project_members pm on pm.project_id = b.project_id and pm.user_id = $2
		where b.id=$1 and (p.owner_user_id = $2 or pm.user_id is not null)
	`, boardID, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Helpers for API layer to resolve board/list relationships for events
func (s *Store) BoardIDByList(ctx context.Context, listID int64) (int64, error) {
	var boardID int64
	err := s.db.QueryRowContext(ctx, `select board_id from lists where id=$1`, listID).Scan(&boardID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return boardID, err
}

func (s *Store) BoardAndListByCard(ctx context.Context, cardID int64) (int64, int64, error) {
	var boardID, listID int64
	err := s.db.QueryRowContext(ctx, `select l.board_id, c.list_id from cards c join lists l on l.id=c.list_id where c.id=$1`, cardID).
		Scan(&boardID, &listID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	return boardID, listID, err
}

func (s *Store) CommentsByCard(ctx context.Context, cardID int64) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		`select id, card_id, body, created_at from comments where card_id=$1 order by id`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.CardID, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AddComment(ctx context.Context, cardID int64, body string) (Comment, error) {
	var c Comment
	err := s.db.QueryRowContext(ctx,
		`insert into comments(card_id, body) values($1, $2) returning id, card_id, body, created_at`,
		cardID, body,
	).Scan(&c.ID, &c.CardID, &c.Body, &c.CreatedAt)
	return c, err
}

// Auth & Users
func (s *Store) CreateUser(ctx context.Context, email, passwordHash, name string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `insert into users(email, password_hash, name) values($1,$2,$3)
		returning id, email, name, coalesce(avatar_url,''), is_active, is_admin, created_at`, email, passwordHash, name).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// get user creds by email, including password hash
func (s *Store) userCredsByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	var verified bool
	err := s.db.QueryRowContext(ctx, `select id, email, name, coalesce(avatar_url,''), is_active, is_admin, created_at, password_hash, coalesce(email_verified,false)
		from users where lower(email)=lower($1)`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &hash, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	if err == nil && !verified {
		return User{}, "", errors.New("email_not_verified")
	}
	return u, hash, err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, time.Time, error) {
	// 32 random bytes, base64 URL encoded
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	expires := time.Now().Add(ttl)
	_, err := s.db.ExecContext(ctx, `insert into sessions(user_id, token, expires_at) values($1,$2,$3)`, userID, token, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) UserBySession(ctx context.Context, token string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `select u.id, u.email, u.name, coalesce(u.avatar_url,''), u.is_active, u.is_admin, coalesce(u.email_verified,false), u.created_at
		from sessions s join users u on u.id=s.user_id
		where s.token=$1 and s.expires_at > now()`, token).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.EmailVerified, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `delete from sessions where token=$1`, token)
	return err
}

// Verify user password and return user if ok
func (s *Store) Authenticate(ctx context.Context, email, password string) (User, error) {
	u, hash, err := s.userCredsByEmail(ctx, email)
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrNotFound
	}
	if !u.IsActive {
		return User{}, errors.New("user_inactive")
	}
	return u, nil
}

// UpdateUserPasswordByEmail sets a new bcrypt-hashed password for a user identified by email.
// If the user doesn't exist, it's a no-op to avoid leaking existence.
func (s *Store) UpdateUserPasswordByEmail(ctx context.Context, email, newPassword string) error {
	if strings.TrimSpace(email) == "" || strings.TrimSpace(newPassword) == "" {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `update users set password_hash=$1 where lower(email)=lower($2)`, string(hash), email)
	if err != nil {
		return err
	}
	return nil
}

// MarkEmailVerified sets users.email_verified=true by email (case-insensitive)
func (s *Store) MarkEmailVerified(ctx context.Context, email string) error {
	if strings.TrimSpace(email) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `update users set email_verified=true where lower(email)=lower($1)`, email)
	return err
}

// get user by email (without password hash)
func (s *Store) userByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `select id, email, name, coalesce(avatar_url,''), is_active, is_admin, created_at from users where lower(email)=lower($1)`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// EnsureOAuthUser links or creates a user for given provider and provider_user_id, returns the user
func (s *Store) EnsureOAuthUser(ctx context.Context, provider, providerUserID, email, name string) (User, error) {
	// 1) Try find by oauth_accounts
	var u User
	err := s.db.QueryRowContext(ctx, `select u.id, u.email, u.name, coalesce(u.avatar_url,''), u.is_active, u.is_admin, u.created_at
		from oauth_accounts oa join users u on u.id = oa.user_id
		where oa.provider=$1 and oa.provider_user_id=$2`, provider, providerUserID).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt)
	switch {
	case err == nil:
		return u, nil
	case !errors.Is(err, sql.ErrNoRows) && err != nil:
		return User{}, err
	}
	// 2) Try find user by email
	haveUser, err := s.userByEmail(ctx, email)
	notFound := errors.Is(err, ErrNotFound)
	if err != nil && !notFound {
		return User{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if notFound {
		// Create user
		err = tx.QueryRowContext(ctx, `insert into users(email, password_hash, name, email_verified) values($1,$2,$3,true)
				returning id, email, name, coalesce(avatar_url,''), is_active, is_admin, created_at`, email, "", name).
			Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt)
		if err != nil {
			return User{}, err
		}
	} else {
		u = haveUser
	}
	// 3) Link oauth account (ignore duplicate unique constraint)
	if _, err = tx.ExecContext(ctx, `insert into oauth_accounts(user_id, provider, provider_user_id) values($1,$2,$3)
			on conflict (provider, provider_user_id) do nothing`, u.ID, provider, providerUserID); err != nil {
		return User{}, err
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	return u, nil
}

// DeleteUser removes a user and nullifies ownership references to satisfy FKs.
// It sets projects.owner_user_id = NULL for projects owned by the user, then deletes the user.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `update projects set owner_user_id = null where owner_user_id = $1`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `delete from users where id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) UpdateCard(ctx context.Context, id int64, title *string, description *string, pos *int64, dueAt *time.Time, descriptionIsMD *bool, assigneeUserID *int64) error {
	q := "update cards set "
	args := []any{}
	idx := 1
	set := []string{}
	if title != nil {
		set = append(set, fmt.Sprintf("title=$%d", idx))
		args = append(args, *title)
		idx++
	}
	if description != nil {
		set = append(set, fmt.Sprintf("description=$%d", idx))
		args = append(args, *description)
		idx++
	}
	if pos != nil {
		set = append(set, fmt.Sprintf("pos=$%d", idx))
		args = append(args, *pos)
		idx++
	}
	if dueAt != nil {
		set = append(set, fmt.Sprintf("due_at=$%d", idx))
		args = append(args, *dueAt)
		idx++
	}
	if descriptionIsMD != nil {
		set = append(set, fmt.Sprintf("description_is_md=$%d", idx))
		args = append(args, *descriptionIsMD)
		idx++
	}
	if assigneeUserID != nil {
		set = append(set, fmt.Sprintf("assignee_user_id=$%d", idx))
		args = append(args, *assigneeUserID)
		idx++
	}
	if len(set) == 0 {
		return nil
	}
	q += fmt.Sprintf("%s where id=$%d", joinComma(set), idx)
	args = append(args, id)
	_, err := s.db.ExecContext(ctx, q, args...)
	return err
}

func (s *Store) MoveCard(ctx context.Context, cardID int64, targetList int64, newIndex int) error {
	attempts := 0
retry:
	var listID int64
	var pos int64
	if err := s.db.QueryRowContext(ctx, `select list_id, pos from cards where id=$1`, cardID).Scan(&listID, &pos); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if targetList != listID {
		if _, err = tx.ExecContext(ctx, `update cards set list_id=$1 where id=$2`, targetList, cardID); err != nil {
			_ = tx.Rollback()
			return err
		}
		listID = targetList
	}

	rows, err := tx.QueryContext(ctx,
		`select pos from cards where list_id=$1 and id<>$2 order by pos, id`, listID, cardID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer rows.Close()
	var positions []int64
	for rows.Next() {
		var p int64
		if err = rows.Scan(&p); err != nil {
			_ = tx.Rollback()
			return err
		}
		positions = append(positions, p)
	}
	if err = rows.Err(); err != nil {
		_ = tx.Rollback()
		return err
	}

	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex > len(positions) {
		newIndex = len(positions)
	}

	var beforePos, afterPos *int64
	if newIndex > 0 {
		v := positions[newIndex-1]
		beforePos = &v
	}
	if newIndex < len(positions) {
		v := positions[newIndex]
		afterPos = &v
	}

	var newPos int64
	switch {
	case beforePos == nil && afterPos == nil:
		newPos = 1000
	case beforePos != nil && afterPos == nil:
		newPos = *beforePos + 1000
	case beforePos == nil && afterPos != nil:
		newPos = *afterPos - 500
		if newPos <= 0 {
			newPos = 1
		}
	default:
		gap := (*afterPos - *beforePos)
		if gap <= 1 {
			if err = renumberPositions(ctx, tx, listID); err != nil {
				_ = tx.Rollback()
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			attempts++
			if attempts < 2 {
				goto retry
			}
			return errors.New("move failed after renumber")
		}
		newPos = *beforePos + gap/2
	}

	if _, err = tx.ExecContext(ctx, `update cards set pos=$1 where id=$2`, newPos, cardID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// MoveList moves a list within its board or to another board at given index
func (s *Store) MoveList(ctx context.Context, listID int64, targetBoardID int64, newIndex int) error {
	attempts := 0
retry:
	var boardID int64
	var pos int64
	if err := s.db.QueryRowContext(ctx, `select board_id, pos from lists where id=$1`, listID).Scan(&boardID, &pos); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// change board if requested
	if targetBoardID != 0 && targetBoardID != boardID {
		if _, err = tx.ExecContext(ctx, `update lists set board_id=$1 where id=$2`, targetBoardID, listID); err != nil {
			_ = tx.Rollback()
			return err
		}
		boardID = targetBoardID
	}
	rows, err := tx.QueryContext(ctx, `select pos from lists where board_id=$1 and id<>$2 order by pos, id`, boardID, listID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer rows.Close()
	var positions []int64
	for rows.Next() {
		var p int64
		if err = rows.Scan(&p); err != nil {
			_ = tx.Rollback()
			return err
		}
		positions = append(positions, p)
	}
	if err = rows.Err(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex > len(positions) {
		newIndex = len(positions)
	}
	var beforePos, afterPos *int64
	if newIndex > 0 {
		v := positions[newIndex-1]
		beforePos = &v
	}
	if newIndex < len(positions) {
		v := positions[newIndex]
		afterPos = &v
	}
	var newPos int64
	switch {
	case beforePos == nil && afterPos == nil:
		newPos = 1000
	case beforePos != nil && afterPos == nil:
		newPos = *beforePos + 1000
	case beforePos == nil && afterPos != nil:
		newPos = *afterPos - 500
		if newPos <= 0 {
			newPos = 1
		}
	default:
		gap := (*afterPos - *beforePos)
		if gap <= 1 {
			if err = renumberListPositions(ctx, tx, boardID); err != nil {
				_ = tx.Rollback()
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			attempts++
			if attempts < 2 {
				goto retry
			}
			return errors.New("move list failed after renumber")
		}
		newPos = *beforePos + gap/2
	}
	if _, err = tx.ExecContext(ctx, `update lists set pos=$1 where id=$2`, newPos, listID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// MoveBoard reorders a board among all boards
func (s *Store) MoveBoard(ctx context.Context, boardID int64, newIndex int) error {
	attempts := 0
retry:
	var pos int64
	if err := s.db.QueryRowContext(ctx, `select pos from boards where id=$1`, boardID).Scan(&pos); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `select pos from boards where id<>$1 order by pos, id`, boardID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer rows.Close()
	var positions []int64
	for rows.Next() {
		var p int64
		if err = rows.Scan(&p); err != nil {
			_ = tx.Rollback()
			return err
		}
		positions = append(positions, p)
	}
	if err = rows.Err(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex > len(positions) {
		newIndex = len(positions)
	}
	var beforePos, afterPos *int64
	if newIndex > 0 {
		v := positions[newIndex-1]
		beforePos = &v
	}
	if newIndex < len(positions) {
		v := positions[newIndex]
		afterPos = &v
	}
	var newPos int64
	switch {
	case beforePos == nil && afterPos == nil:
		newPos = 1000
	case beforePos != nil && afterPos == nil:
		newPos = *beforePos + 1000
	case beforePos == nil && afterPos != nil:
		newPos = *afterPos - 500
		if newPos <= 0 {
			newPos = 1
		}
	default:
		gap := (*afterPos - *beforePos)
		if gap <= 1 {
			if err = renumberBoardPositions(ctx, tx); err != nil {
				_ = tx.Rollback()
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			attempts++
			if attempts < 2 {
				goto retry
			}
			return errors.New("move board failed after renumber")
		}
		newPos = *beforePos + gap/2
	}
	if _, err = tx.ExecContext(ctx, `update boards set pos=$1 where id=$2`, newPos, boardID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func renumberPositions(ctx context.Context, tx *sql.Tx, listID int64) error {
	rows, err := tx.QueryContext(ctx, `select id from cards where list_id=$1 order by pos, id`, listID)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	pos := int64(1000)
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update cards set pos=$1 where id=$2`, pos, id); err != nil {
			return err
		}
		pos += 1000
	}
	return nil
}

func renumberListPositions(ctx context.Context, tx *sql.Tx, boardID int64) error {
	rows, err := tx.QueryContext(ctx, `select id from lists where board_id=$1 order by pos, id`, boardID)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	pos := int64(1000)
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update lists set pos=$1 where id=$2`, pos, id); err != nil {
			return err
		}
		pos += 1000
	}
	return nil
}

func renumberBoardPositions(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `select id from boards order by pos, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	pos := int64(1000)
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update boards set pos=$1 where id=$2`, pos, id); err != nil {
			return err
		}
		pos += 1000
	}
	return nil
}

var ErrNotFound = errors.New("not found")

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += ", " + parts[i]
	}
	return out
}

const schema = `
create table if not exists boards(
    id bigserial primary key,
    title text not null check (length(title) > 0),
	color text,
    created_at timestamptz not null default now()
);
alter table boards add column if not exists pos bigint not null default 1000;
alter table boards add column if not exists color text;
-- upcoming: projects linkage and creator
alter table boards add column if not exists project_id bigint;
alter table boards add column if not exists created_by bigint;
-- indexes and fks will be added after projects/users tables exist
create index if not exists boards_pos_idx on boards(pos);
create table if not exists lists(
    id bigserial primary key,
    board_id bigint not null references boards(id) on delete cascade,
    title text not null check (length(title) > 0),
	color text,
    pos bigint not null default 1000,
    created_at timestamptz not null default now()
);
alter table lists add column if not exists color text;
create index if not exists lists_board_idx on lists(board_id);
create table if not exists cards(
    id bigserial primary key,
    list_id bigint not null references lists(id) on delete cascade,
    title text not null check (length(title) > 0),
    description text not null default '',
	color text,
    pos bigint not null default 1000,
    due_at timestamptz,
    created_at timestamptz not null default now()
);
alter table cards add column if not exists color text;
alter table cards add column if not exists description_is_md boolean not null default false;
-- upcoming: assignee for cards
alter table cards add column if not exists assignee_user_id bigint;
create index if not exists cards_list_idx on cards(list_id);
create table if not exists comments(
    id bigserial primary key,
    card_id bigint not null references cards(id) on delete cascade,
    body text not null check (length(body) > 0),
    created_at timestamptz not null default now()
);
-- upcoming: author of comment
alter table comments add column if not exists user_id bigint;

-- Users and auth
create table if not exists users(
		id bigserial primary key,
		email text unique not null,
		password_hash text not null default '',
		name text not null default '',
		avatar_url text,
		is_active boolean not null default true,
		is_admin boolean not null default false,
		created_at timestamptz not null default now()
);

-- ensure email_verified exists for older installs
alter table users add column if not exists email_verified boolean not null default false;

create table if not exists oauth_accounts(
		id bigserial primary key,
		user_id bigint not null references users(id) on delete cascade,
		provider text not null,
		provider_user_id text not null,
		access_token text,
		refresh_token text,
		expires_at timestamptz,
		unique(provider, provider_user_id)
);

create table if not exists sessions(
		id bigserial primary key,
		user_id bigint not null references users(id) on delete cascade,
		token text unique not null,
		created_at timestamptz not null default now(),
		expires_at timestamptz not null
);

-- Groups
create table if not exists groups(
		id bigserial primary key,
		name text unique not null,
		created_at timestamptz not null default now()
);
create table if not exists user_groups(
		user_id bigint not null references users(id) on delete cascade,
		group_id bigint not null references groups(id) on delete cascade,
		role smallint not null default 1,
		primary key(user_id, group_id)
);
-- ensure role column exists for older installs
alter table user_groups add column if not exists role smallint not null default 1;

-- Projects and membership
create table if not exists projects(
		id bigserial primary key,
		name text not null,
		owner_user_id bigint references users(id),
		created_at timestamptz not null default now()
);
create table if not exists project_members(
		project_id bigint not null references projects(id) on delete cascade,
		user_id bigint not null references users(id) on delete cascade,
		role smallint not null default 2,
		primary key(project_id, user_id)
);

-- Boards to groups mapping (visibility of boards for groups)
create table if not exists board_groups(
	board_id bigint not null references boards(id) on delete cascade,
	group_id bigint not null references groups(id) on delete cascade,
	primary key(board_id, group_id)
);

-- Link boards.project_id to projects.id, created_by to users.id if tables exist
do $$ begin
	if exists (select 1 from information_schema.tables where table_name='projects') then
		begin
			alter table boards
				add constraint boards_project_fk foreign key (project_id) references projects(id) on delete set null;
		exception when duplicate_object then null; end;
	end if;
	if exists (select 1 from information_schema.tables where table_name='users') then
		begin
			alter table boards
				add constraint boards_created_by_fk foreign key (created_by) references users(id) on delete set null;
			alter table cards
				add constraint cards_assignee_fk foreign key (assignee_user_id) references users(id) on delete set null;
			alter table comments
				add constraint comments_user_fk foreign key (user_id) references users(id) on delete set null;
		exception when duplicate_object then null; end;
	end if;
end $$;
`
