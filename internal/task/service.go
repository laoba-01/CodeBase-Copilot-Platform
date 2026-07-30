package task

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/domain"
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Enqueue(ctx context.Context, repoID string, taskType domain.TaskType) (*domain.Task, error) {
	t := &domain.Task{
		ID:     uuid.New().String(),
		RepoID: repoID,
		Type:   taskType,
		Status: domain.TaskStatusPending,
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO tasks (id, repo_id, type, status, progress) VALUES ($1,$2,$3,$4,0)
	`, t.ID, t.RepoID, t.Type, t.Status)
	if err != nil {
		return nil, fmt.Errorf("enqueue task: %w", err)
	}
	return t, nil
}

func (s *Service) UpdateProgress(ctx context.Context, taskID string, progress int) error {
	_, err := s.db.Exec(ctx,
		`UPDATE tasks SET progress = $1, updated_at = now() WHERE id = $2`, progress, taskID)
	return err
}

func (s *Service) Complete(ctx context.Context, taskID, result string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE tasks SET status = 'completed', result = $1, progress = 100, updated_at = now() WHERE id = $2`,
		result, taskID)
	return err
}

func (s *Service) Fail(ctx context.Context, taskID, errMsg string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE tasks SET status = 'failed', error = $1, updated_at = now() WHERE id = $2`,
		errMsg, taskID)
	return err
}

func (s *Service) List(ctx context.Context, repoID string) ([]domain.Task, error) {
	var rows pgx.Rows
	var err error
	if repoID != "" {
		rows, err = s.db.Query(ctx,
			`SELECT id, repo_id, type, status, progress, COALESCE(error,''), COALESCE(result,''), created_at, updated_at
			 FROM tasks WHERE repo_id = $1 ORDER BY created_at DESC`, repoID)
	} else {
		rows, err = s.db.Query(ctx,
			`SELECT id, repo_id, type, status, progress, COALESCE(error,''), COALESCE(result,''), created_at, updated_at
			 FROM tasks ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.RepoID, &t.Type, &t.Status, &t.Progress,
			&t.Error, &t.Result, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Service) Get(ctx context.Context, taskID string) (*domain.Task, error) {
	var t domain.Task
	err := s.db.QueryRow(ctx,
		`SELECT id, repo_id, type, status, progress, COALESCE(error,''), COALESCE(result,''), created_at, updated_at FROM tasks WHERE id = $1`,
		taskID,
	).Scan(&t.ID, &t.RepoID, &t.Type, &t.Status, &t.Progress, &t.Error, &t.Result, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &t, nil
}
