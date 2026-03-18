package store

import (
	"context"
	"database/sql"
)

type AssigneeStore struct {
	db *sql.DB
}

func NewAssigneeStore(db *sql.DB) *AssigneeStore {
	return &AssigneeStore{db: db}
}

func (s *AssigneeStore) Add(ctx context.Context, taskID, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_assignees (task_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, taskID, userID)
	return err
}

func (s *AssigneeStore) Remove(ctx context.Context, taskID, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM task_assignees
		WHERE task_id = $1 AND user_id = $2
	`, taskID, userID)
	return err
}

func (s *AssigneeStore) List(ctx context.Context, taskID string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id::text, u.name, u.email, u.created_at
		FROM task_assignees ta
		JOIN users u ON u.id = ta.user_id
		WHERE ta.task_id = $1
		ORDER BY u.name
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
