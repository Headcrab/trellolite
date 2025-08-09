package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) ListBoards(ctx context.Context) ([]Board, error) {
	rows, err := s.db.QueryContext(ctx, `select id, title, coalesce(color,'') as color, created_at from boards order by pos, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.Title, &b.Color, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) CreateBoard(ctx context.Context, title string) (Board, error) {
	var next int64 = 1000
	_ = s.db.QueryRowContext(ctx, `select coalesce(max(pos),0)+1000 from boards`).Scan(&next)
	var b Board
	err := s.db.QueryRowContext(ctx, `insert into boards(title, pos) values($1,$2) returning id, title, coalesce(color,''), created_at`, title, next).
		Scan(&b.ID, &b.Title, &b.Color, &b.CreatedAt)
	return b, err
}

func (s *Store) GetBoard(ctx context.Context, id int64) (Board, error) {
	var b Board
	err := s.db.QueryRowContext(ctx, `select id, title, coalesce(color,''), created_at from boards where id=$1`, id).
		Scan(&b.ID, &b.Title, &b.Color, &b.CreatedAt)
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
		`select id, list_id, title, description, coalesce(color,''), pos, due_at, created_at from cards where list_id=$1 order by pos, id`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.ListID, &c.Title, &c.Description, &c.Color, &c.Pos, &c.DueAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateCard(ctx context.Context, listID int64, title, description string) (Card, error) {
	var next int64 = 1000
	_ = s.db.QueryRowContext(ctx, `select coalesce(max(pos),0)+1000 from cards where list_id=$1`, listID).Scan(&next)
	var c Card
	err := s.db.QueryRowContext(ctx,
		`insert into cards(list_id, title, description, pos) values($1,$2,$3,$4)
		 returning id, list_id, title, description, coalesce(color,''), pos, due_at, created_at`,
		listID, title, description, next).
		Scan(&c.ID, &c.ListID, &c.Title, &c.Description, &c.Color, &c.Pos, &c.DueAt, &c.CreatedAt)
	return c, err
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

func (s *Store) UpdateCard(ctx context.Context, id int64, title *string, description *string, pos *int64, dueAt *time.Time) error {
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
	if len(set) == 0 {
		return nil
	}
	q += fmt.Sprintf("%s where id=$%d", joinComma(set), idx)
	args = append(args, id)
	_, err := s.db.ExecContext(ctx, q, args...)
	return err
}

func (s *Store) MoveCard(ctx context.Context, cardID int64, targetList int64, newIndex int) error {
	for attempt := 0; attempt < 2; attempt++ {
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

		var beforePos *int64
		var afterPos *int64
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
				return s.MoveCard(ctx, cardID, listID, newIndex)
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
	return errors.New("move failed after renumber")
}

// MoveList moves a list within its board or to another board at given index
func (s *Store) MoveList(ctx context.Context, listID int64, targetBoardID int64, newIndex int) error {
	for attempt := 0; attempt < 2; attempt++ {
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
				return s.MoveList(ctx, listID, boardID, newIndex)
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
	return errors.New("move list failed after renumber")
}

// MoveBoard reorders a board among all boards
func (s *Store) MoveBoard(ctx context.Context, boardID int64, newIndex int) error {
	for attempt := 0; attempt < 2; attempt++ {
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
				return s.MoveBoard(ctx, boardID, newIndex)
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
	return errors.New("move board failed after renumber")
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
create index if not exists cards_list_idx on cards(list_id);
create table if not exists comments(
    id bigserial primary key,
    card_id bigint not null references cards(id) on delete cascade,
    body text not null check (length(body) > 0),
    created_at timestamptz not null default now()
);
`
