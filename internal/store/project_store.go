package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectStore struct {
	db *sql.DB
}

func NewProjectStore(db *sql.DB) *ProjectStore {
	return &ProjectStore{db: db}
}

func (s *ProjectStore) Create(ctx context.Context, name string) (Project, error) {
	var project Project
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO projects (name)
		VALUES ($1)
		RETURNING id::text, name, created_at
	`, name)

	if err := row.Scan(&project.ID, &project.Name, &project.CreatedAt); err != nil {
		return project, err
	}
	return project, nil
}

func (s *ProjectStore) Get(ctx context.Context, id string) (Project, error) {
	var project Project
	row := s.db.QueryRowContext(ctx, `
		SELECT id::text, name, created_at
		FROM projects
		WHERE id = $1
	`, id)

	if err := row.Scan(&project.ID, &project.Name, &project.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project, ErrNotFound
		}
		return project, err
	}
	return project, nil
}

func (s *ProjectStore) List(ctx context.Context, limit, offset int) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, name, created_at
		FROM projects
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Name, &project.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}
