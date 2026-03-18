package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("not found")

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    int        `json:"priority"`
	Status      string     `json:"status"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	ProjectID   *string    `json:"project_id,omitempty"`
	Tags        []string   `json:"tags"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

type CreateTaskInput struct {
	Title       string
	Description string
	Priority    int
	DueDate     *time.Time
	ProjectID   *string
	Tags        []string
}

type UpdateTaskInput struct {
	Title         *string
	Description   *string
	Priority      *int
	DueDate       *time.Time
	ProjectID     *string
	Tags          *[]string
	ClearDueDate  bool
	ClearProject  bool
}

type ListFilters struct {
	Status    string
	Priority  *int
	ProjectID string
	Search    string
	Tag       string
}

func (s *TaskStore) Create(ctx context.Context, input CreateTaskInput) (Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var task Task
	row := tx.QueryRowContext(ctx, `
		INSERT INTO tasks (title, description, priority, due_date, project_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, title, description, priority, status,
		          due_date, project_id::text, created_at, updated_at
	`, input.Title, input.Description, input.Priority, input.DueDate, input.ProjectID)

	var dueDate sql.NullTime
	var projectID sql.NullString
	if err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Priority, &task.Status,
		&dueDate, &projectID, &task.CreatedAt, &task.UpdatedAt); err != nil {
		return Task{}, err
	}
	task.DueDate = nullTimePtr(dueDate)
	task.ProjectID = nullStringPtr(projectID)

	if len(input.Tags) > 0 {
		if err := attachTags(ctx, tx, task.ID, input.Tags); err != nil {
			return Task{}, err
		}
		task.Tags = normalizeTags(input.Tags)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *TaskStore) Get(ctx context.Context, id string) (Task, error) {
	var task Task
	row := s.db.QueryRowContext(ctx, `
		SELECT id::text, title, description, priority, status, due_date, project_id::text, created_at, updated_at
		FROM tasks
		WHERE id = $1
	`, id)

	var dueDate sql.NullTime
	var projectID sql.NullString
	if err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Priority, &task.Status,
		&dueDate, &projectID, &task.CreatedAt, &task.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return task, ErrNotFound
		}
		return task, err
	}
	task.DueDate = nullTimePtr(dueDate)
	task.ProjectID = nullStringPtr(projectID)

	tags, err := s.tagsForTask(ctx, id)
	if err != nil {
		return task, err
	}
	task.Tags = tags
	return task, nil
}

func (s *TaskStore) List(ctx context.Context, filters ListFilters, limit, offset int) ([]Task, error) {
	query := `
		SELECT id::text, title, description, priority, status, due_date, project_id::text, created_at, updated_at
		FROM tasks
	`
	where := []string{}
	args := []any{}
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filters.Status != "" {
		where = append(where, "status = "+arg(filters.Status))
	}
	if filters.Priority != nil {
		where = append(where, "priority = "+arg(*filters.Priority))
	}
	if filters.ProjectID != "" {
		where = append(where, "project_id = "+arg(filters.ProjectID))
	}
	if filters.Search != "" {
		search := "%" + strings.ToLower(filters.Search) + "%"
		where = append(where, "(LOWER(title) LIKE "+arg(search)+" OR LOWER(description) LIKE "+arg(search)+")")
	}
	if filters.Tag != "" {
		where = append(where, `EXISTS (
			SELECT 1 FROM task_tags tt
			JOIN tags t ON t.id = tt.tag_id
			WHERE tt.task_id = tasks.id AND t.name = `+arg(strings.ToLower(filters.Tag))+`
		)`)
	}

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	query += " ORDER BY created_at DESC LIMIT " + arg(limit) + " OFFSET " + arg(offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	var taskIDs []string
	for rows.Next() {
		var task Task
		var dueDate sql.NullTime
		var projectID sql.NullString
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Priority, &task.Status,
			&dueDate, &projectID, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		task.DueDate = nullTimePtr(dueDate)
		task.ProjectID = nullStringPtr(projectID)
		tasks = append(tasks, task)
		taskIDs = append(taskIDs, task.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return tasks, nil
	}

	tagMap, err := s.tagsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Tags = tagMap[tasks[i].ID]
	}

	return tasks, nil
}

func (s *TaskStore) UpdateStatus(ctx context.Context, id string, status string) (Task, error) {
	var task Task
	row := s.db.QueryRowContext(ctx, `
		UPDATE tasks
		SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id::text, title, description, priority, status, due_date, project_id::text, created_at, updated_at
	`, id, status)

	var dueDate sql.NullTime
	var projectID sql.NullString
	if err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Priority, &task.Status,
		&dueDate, &projectID, &task.CreatedAt, &task.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return task, ErrNotFound
		}
		return task, err
	}
	task.DueDate = nullTimePtr(dueDate)
	task.ProjectID = nullStringPtr(projectID)

	tags, err := s.tagsForTask(ctx, id)
	if err != nil {
		return task, err
	}
	task.Tags = tags
	return task, nil
}

func (s *TaskStore) Update(ctx context.Context, id string, input UpdateTaskInput) (Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	set := []string{}
	args := []any{}
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if input.Title != nil {
		set = append(set, "title = "+arg(*input.Title))
	}
	if input.Description != nil {
		set = append(set, "description = "+arg(*input.Description))
	}
	if input.Priority != nil {
		set = append(set, "priority = "+arg(*input.Priority))
	}
	if input.ClearDueDate {
		set = append(set, "due_date = NULL")
	} else if input.DueDate != nil {
		set = append(set, "due_date = "+arg(*input.DueDate))
	}
	if input.ClearProject {
		set = append(set, "project_id = NULL")
	} else if input.ProjectID != nil {
		set = append(set, "project_id = "+arg(*input.ProjectID))
	}

	set = append(set, "updated_at = now()")
	if len(set) == 0 {
		return s.Get(ctx, id)
	}

	query := `
		UPDATE tasks
		SET ` + strings.Join(set, ", ") + `
		WHERE id = ` + arg(id) + `
		RETURNING id::text, title, description, priority, status, due_date, project_id::text, created_at, updated_at
	`

	var task Task
	var dueDate sql.NullTime
	var projectID sql.NullString
	row := tx.QueryRowContext(ctx, query, args...)
	if err := row.Scan(&task.ID, &task.Title, &task.Description, &task.Priority, &task.Status,
		&dueDate, &projectID, &task.CreatedAt, &task.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return task, ErrNotFound
		}
		return task, err
	}
	task.DueDate = nullTimePtr(dueDate)
	task.ProjectID = nullStringPtr(projectID)

	if input.Tags != nil {
		if err := replaceTags(ctx, tx, task.ID, *input.Tags); err != nil {
			return Task{}, err
		}
		task.Tags = normalizeTags(*input.Tags)
	} else {
		tags, err := s.tagsForTask(ctx, task.ID)
		if err != nil {
			return Task{}, err
		}
		task.Tags = tags
	}

	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *TaskStore) Stats(ctx context.Context) (map[string]int, int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM tasks
		GROUP BY status
	`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	byStatus := map[string]int{
		"open":        0,
		"in_progress": 0,
		"done":        0,
	}

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, 0, err
		}
		byStatus[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var overdue int
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE due_date IS NOT NULL AND status != 'done' AND due_date < now()
	`)
	if err := row.Scan(&overdue); err != nil {
		return nil, 0, err
	}

	return byStatus, overdue, nil
}

func (s *TaskStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return err
}

func (s *TaskStore) ListOverdue(ctx context.Context, limit, offset int) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, title, description, priority, status, due_date, project_id::text, created_at, updated_at
		FROM tasks
		WHERE due_date IS NOT NULL AND status != 'done' AND due_date < now()
		ORDER BY due_date ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTasks(ctx, rows)
}

func (s *TaskStore) ListDueSoon(ctx context.Context, days int, limit, offset int) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, title, description, priority, status, due_date, project_id::text, created_at, updated_at
		FROM tasks
		WHERE due_date IS NOT NULL AND status != 'done' AND due_date <= now() + ($1 || ' days')::interval
		ORDER BY due_date ASC
		LIMIT $2 OFFSET $3
	`, days, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanTasks(ctx, rows)
}

func (s *TaskStore) scanTasks(ctx context.Context, rows *sql.Rows) ([]Task, error) {
	var tasks []Task
	var taskIDs []string
	for rows.Next() {
		var task Task
		var dueDate sql.NullTime
		var projectID sql.NullString
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Priority, &task.Status,
			&dueDate, &projectID, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		task.DueDate = nullTimePtr(dueDate)
		task.ProjectID = nullStringPtr(projectID)
		tasks = append(tasks, task)
		taskIDs = append(taskIDs, task.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return tasks, nil
	}

	tagMap, err := s.tagsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Tags = tagMap[tasks[i].ID]
	}
	return tasks, nil
}

func (s *TaskStore) BulkUpdateStatus(ctx context.Context, ids []string, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, status)
	for i, id := range ids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+2))
		args = append(args, id)
	}

	query := `
		UPDATE tasks
		SET status = $1, updated_at = now()
		WHERE id IN (` + strings.Join(placeholders, ", ") + `)
	`
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *TaskStore) ProjectStats(ctx context.Context, projectID string) (map[string]any, error) {
	var exists string
	if err := s.db.QueryRowContext(ctx, `SELECT id::text FROM projects WHERE id = $1`, projectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM tasks
		WHERE project_id = $1
		GROUP BY status
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byStatus := map[string]int{
		"open":        0,
		"in_progress": 0,
		"done":        0,
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		byStatus[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var overdue int
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE project_id = $1 AND due_date IS NOT NULL AND status != 'done' AND due_date < now()
	`, projectID)
	if err := row.Scan(&overdue); err != nil {
		return nil, err
	}

	return map[string]any{
		"project_id": projectID,
		"by_status":  byStatus,
		"overdue":    overdue,
	}, nil
}

func (s *TaskStore) tagsForTask(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.name
		FROM tags t
		JOIN task_tags tt ON tt.tag_id = t.id
		WHERE tt.task_id = $1
		ORDER BY t.name
	`, id)
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

func (s *TaskStore) tagsForTasks(ctx context.Context, ids []string) (map[string][]string, error) {
	if len(ids) == 0 {
		return map[string][]string{}, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
		args = append(args, id)
	}

	query := `
		SELECT tt.task_id::text, t.name
		FROM task_tags tt
		JOIN tags t ON t.id = tt.tag_id
		WHERE tt.task_id IN (` + strings.Join(placeholders, ", ") + `)
		ORDER BY t.name
	`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tagMap := make(map[string][]string, len(ids))
	for rows.Next() {
		var taskID string
		var name string
		if err := rows.Scan(&taskID, &name); err != nil {
			return nil, err
		}
		tagMap[taskID] = append(tagMap[taskID], name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tagMap, nil
}

func attachTags(ctx context.Context, tx *sql.Tx, taskID string, tags []string) error {
	for _, tag := range normalizeTags(tags) {
		var tagID string
		row := tx.QueryRowContext(ctx, `
			INSERT INTO tags (name)
			VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text
		`, tag)
		if err := row.Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_tags (task_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, taskID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func replaceTags(ctx context.Context, tx *sql.Tx, taskID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = $1`, taskID); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	return attachTags(ctx, tx, taskID, tags)
}

func normalizeTags(tags []string) []string {
	unique := map[string]struct{}{}
	for _, tag := range tags {
		trimmed := strings.ToLower(strings.TrimSpace(tag))
		if trimmed == "" {
			continue
		}
		unique[trimmed] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for tag := range unique {
		result = append(result, tag)
	}
	sortStrings(result)
	return result
}

func sortStrings(values []string) {
	if len(values) < 2 {
		return
	}
	for i := 0; i < len(values)-1; i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func nullStringPtr(value sql.NullString) *string {
	if value.Valid {
		return &value.String
	}
	return nil
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if value.Valid {
		return &value.Time
	}
	return nil
}
