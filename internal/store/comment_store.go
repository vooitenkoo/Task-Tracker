package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Comment struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type CommentStore struct {
	db *sql.DB
}

func NewCommentStore(db *sql.DB) *CommentStore {
	return &CommentStore{db: db}
}

func (s *CommentStore) Create(ctx context.Context, taskID, author, body string) (Comment, error) {
	var comment Comment
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO task_comments (task_id, author, body)
		VALUES ($1, $2, $3)
		RETURNING id::text, task_id::text, author, body, created_at
	`, taskID, author, body)

	if err := row.Scan(&comment.ID, &comment.TaskID, &comment.Author, &comment.Body, &comment.CreatedAt); err != nil {
		return comment, err
	}
	return comment, nil
}

func (s *CommentStore) ListByTask(ctx context.Context, taskID string, limit, offset int) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, task_id::text, author, body, created_at
		FROM task_comments
		WHERE task_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, taskID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.TaskID, &comment.Author, &comment.Body, &comment.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return comments, nil
}

func (s *CommentStore) EnsureTaskExists(ctx context.Context, taskID string) error {
	var id string
	row := s.db.QueryRowContext(ctx, `SELECT id::text FROM tasks WHERE id = $1`, taskID)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
