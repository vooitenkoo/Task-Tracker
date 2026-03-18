package store

import (
	"context"
	"database/sql"
)

type ProjectMember struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

type ProjectMemberStore struct {
	db *sql.DB
}

func NewProjectMemberStore(db *sql.DB) *ProjectMemberStore {
	return &ProjectMemberStore{db: db}
}

func (s *ProjectMemberStore) Add(ctx context.Context, projectID, userID, role string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, projectID, userID, role)
	return err
}

func (s *ProjectMemberStore) List(ctx context.Context, projectID string) ([]ProjectMember, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id::text, u.name, u.email, pm.role
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1
		ORDER BY u.name
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []ProjectMember
	for rows.Next() {
		var member ProjectMember
		if err := rows.Scan(&member.UserID, &member.Name, &member.Email, &member.Role); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}
