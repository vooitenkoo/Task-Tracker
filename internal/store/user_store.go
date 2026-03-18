package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, name, email string) (User, error) {
	var user User
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO users (name, email)
		VALUES ($1, $2)
		RETURNING id::text, name, email, created_at
	`, name, email)

	if err := row.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt); err != nil {
		return user, err
	}
	return user, nil
}

func (s *UserStore) Get(ctx context.Context, id string) (User, error) {
	var user User
	row := s.db.QueryRowContext(ctx, `
		SELECT id::text, name, email, created_at
		FROM users
		WHERE id = $1
	`, id)

	if err := row.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, ErrNotFound
		}
		return user, err
	}
	return user, nil
}

func (s *UserStore) List(ctx context.Context, limit, offset int) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, name, email, created_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
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
