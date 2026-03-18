package store

import (
	"context"
	"database/sql"
)

type TagStore struct {
	db *sql.DB
}

func NewTagStore(db *sql.DB) *TagStore {
	return &TagStore{db: db}
}

func (s *TagStore) List(ctx context.Context, limit, offset int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name
		FROM tags
		ORDER BY name
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tags = append(tags, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tags, nil
}
