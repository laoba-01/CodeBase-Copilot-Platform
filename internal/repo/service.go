package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/google/uuid"

	"github.com/codebase-copilot/core/internal/domain"
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, userID, fullName string) (*domain.Repository, error) {
	// Parse owner/repo
	var owner, name string
	if _, err := fmt.Sscanf(fullName, "%s/%s", &owner, &name); err != nil {
		return nil, fmt.Errorf("invalid repo name: %s (expected owner/repo)", fullName)
	}

	// Fetch repo info from GitHub
	ghRepo, err := FetchGitHubRepo("", owner, name) // access_token from user's OAuth
	if err != nil {
		return nil, fmt.Errorf("fetch github repo: %w", err)
	}

	repo := &domain.Repository{
		ID:            uuid.New().String(),
		UserID:        userID,
		Name:          name,
		FullName:      ghRepo.FullName,
		CloneURL:      ghRepo.CloneURL,
		DefaultBranch: ghRepo.DefaultBranch,
		Status:        domain.RepoStatusPending,
		CreatedAt:     time.Now(),
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO repos (id, user_id, name, full_name, clone_url, default_branch, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, repo.ID, repo.UserID, repo.Name, repo.FullName, repo.CloneURL, repo.DefaultBranch, repo.Status)
	if err != nil {
		return nil, fmt.Errorf("insert repo: %w", err)
	}

	return repo, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]domain.Repository, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, name, full_name, clone_url, default_branch, status, indexed_at, created_at
		FROM repos WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}
	defer rows.Close()

	var repos []domain.Repository
	for rows.Next() {
		var r domain.Repository
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.FullName, &r.CloneURL,
			&r.DefaultBranch, &r.Status, &r.IndexedAt, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repos: %w", err)
	}
	return repos, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Repository, error) {
	var r domain.Repository
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, name, full_name, clone_url, default_branch, status, indexed_at, created_at
		FROM repos WHERE id = $1
	`, id).Scan(&r.ID, &r.UserID, &r.Name, &r.FullName, &r.CloneURL,
		&r.DefaultBranch, &r.Status, &r.IndexedAt, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	return &r, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM repos WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	return nil
}
