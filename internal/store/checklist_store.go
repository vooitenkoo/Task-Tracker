package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

type ChecklistItem struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

type ChecklistStore struct {
	db *sql.DB
}

func NewChecklistStore(db *sql.DB) *ChecklistStore {
	return &ChecklistStore{db: db}
}

func (s *ChecklistStore) Create(ctx context.Context, taskID, title string) (ChecklistItem, error) {
	var item ChecklistItem
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO checklist_items (task_id, title)
		VALUES ($1, $2)
		RETURNING id::text, task_id::text, title, done, created_at
	`, taskID, title)

	if err := row.Scan(&item.ID, &item.TaskID, &item.Title, &item.Done, &item.CreatedAt); err != nil {
		return item, err
	}
	return item, nil
}

func (s *ChecklistStore) Update(ctx context.Context, itemID string, title *string, done *bool) (ChecklistItem, error) {
	set := []string{}
	args := []any{}
	addArg := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args))
	}

	if title != nil {
		set = append(set, "title = "+addArg(*title))
	}
	if done != nil {
		set = append(set, "done = "+addArg(*done))
	}
	if len(set) == 0 {
		return s.Get(ctx, itemID)
	}

	args = append(args, itemID)
	query := `
		UPDATE checklist_items
		SET ` + strings.Join(set, ", ") + `
		WHERE id = ` + "$" + strconv.Itoa(len(args)) + `
		RETURNING id::text, task_id::text, title, done, created_at
	`

	var item ChecklistItem
	row := s.db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(&item.ID, &item.TaskID, &item.Title, &item.Done, &item.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return item, ErrNotFound
		}
		return item, err
	}
	return item, nil
}

func (s *ChecklistStore) Get(ctx context.Context, itemID string) (ChecklistItem, error) {
	var item ChecklistItem
	row := s.db.QueryRowContext(ctx, `
		SELECT id::text, task_id::text, title, done, created_at
		FROM checklist_items
		WHERE id = $1
	`, itemID)

	if err := row.Scan(&item.ID, &item.TaskID, &item.Title, &item.Done, &item.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return item, ErrNotFound
		}
		return item, err
	}
	return item, nil
}

func (s *ChecklistStore) Delete(ctx context.Context, itemID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM checklist_items WHERE id = $1`, itemID)
	return err
}

func (s *ChecklistStore) ListByTask(ctx context.Context, taskID string) ([]ChecklistItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, task_id::text, title, done, created_at
		FROM checklist_items
		WHERE task_id = $1
		ORDER BY created_at DESC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ChecklistItem
	for rows.Next() {
		var item ChecklistItem
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Title, &item.Done, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
