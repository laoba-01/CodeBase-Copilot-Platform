package task

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/domain"
)

// ErrAccessDenied is returned when a user attempts to access a task they don't own.
var ErrAccessDenied = errors.New("access denied")

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

func (s *Service) List(ctx context.Context, repoID, userID string) ([]domain.Task, error) {
	var rows pgx.Rows
	var err error
	if repoID != "" {
		// Verify user owns the repo before listing its tasks
		rows, err = s.db.Query(ctx, `
			SELECT t.id, t.repo_id, t.type, t.status, t.progress, COALESCE(t.error,''), COALESCE(t.result,''), t.created_at, t.updated_at
			FROM tasks t
			JOIN repos r ON t.repo_id = r.id
			WHERE t.repo_id = $1 AND r.user_id = $2
			ORDER BY t.created_at DESC
		`, repoID, userID)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT t.id, t.repo_id, t.type, t.status, t.progress, COALESCE(t.error,''), COALESCE(t.result,''), t.created_at, t.updated_at
			FROM tasks t
			JOIN repos r ON t.repo_id = r.id
			WHERE r.user_id = $1
			ORDER BY t.created_at DESC
		`, userID)
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

func (s *Service) Get(ctx context.Context, taskID, userID string) (*domain.Task, error) {
	var t domain.Task
	err := s.db.QueryRow(ctx, `
		SELECT t.id, t.repo_id, t.type, t.status, t.progress, COALESCE(t.error,''), COALESCE(t.result,''), t.created_at, t.updated_at
		FROM tasks t
		JOIN repos r ON t.repo_id = r.id
		WHERE t.id = $1 AND r.user_id = $2
	`, taskID, userID,
	).Scan(&t.ID, &t.RepoID, &t.Type, &t.Status, &t.Progress, &t.Error, &t.Result, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		// Check if task exists but belongs to another user
		var exists bool
		if err2 := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks WHERE id = $1)`, taskID).Scan(&exists); err2 == nil && exists {
			return nil, ErrAccessDenied
		}
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &t, nil
}
