package store

import (
	"context"
	"database/sql"
	"time"
)

type ActivityEntry struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type ActivityStore struct {
	db *sql.DB
}

func NewActivityStore(db *sql.DB) *ActivityStore {
	return &ActivityStore{db: db}
}

func (s *ActivityStore) Add(ctx context.Context, taskID, entryType, message string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_activity (task_id, type, message)
		VALUES ($1, $2, $3)
	`, taskID, entryType, message)
	return err
}

func (s *ActivityStore) ListByTask(ctx context.Context, taskID string, limit, offset int) ([]ActivityEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, task_id::text, type, message, created_at
		FROM task_activity
		WHERE task_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, taskID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var entry ActivityEntry
		if err := rows.Scan(&entry.ID, &entry.TaskID, &entry.Type, &entry.Message, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
