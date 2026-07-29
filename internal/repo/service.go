package repo

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/google/uuid"

	"github.com/codebase-copilot/core/internal/domain"
)

type Service struct {
	db      *pgxpool.Pool
	Indexer func(ctx context.Context, repo *domain.Repository) error
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, userID, fullName string) (*domain.Repository, error) {
	// Parse owner/repo
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid repo name: %s (expected owner/repo)", fullName)
	}
	owner, name := parts[0], parts[1]

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

	// Kick off background indexing if an indexer is wired
	if s.Indexer != nil {
		go func() {
			if err := s.Indexer(context.Background(), repo); err != nil {
				log.Printf("indexer error for repo %s: %v", repo.ID, err)
				// Update repo status to error on failure
				s.db.Exec(context.Background(),
					`UPDATE repos SET status = 'error' WHERE id = $1`, repo.ID)
			}
		}()
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

func (s *Service) Get(ctx context.Context, id, userID string) (*domain.Repository, error) {
	var r domain.Repository
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, name, full_name, clone_url, default_branch, status, indexed_at, created_at
		FROM repos
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&r.ID, &r.UserID, &r.Name, &r.FullName, &r.CloneURL,
		&r.DefaultBranch, &r.Status, &r.IndexedAt, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	return &r, nil
}

func (s *Service) Delete(ctx context.Context, id, userID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM repos WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	return nil
}

// FileNode is a lightweight file-tree entry returned by GetFiles.
type FileNode struct {
	FilePath string `json:"file_path"`
}

// GetFiles returns the list of distinct file paths indexed for a repo (type='file' nodes only).
func (s *Service) GetFiles(ctx context.Context, repoID string) ([]FileNode, error) {
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT file_path FROM index_nodes WHERE repo_id = $1 AND type = 'file' ORDER BY file_path`,
		repoID)
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	defer rows.Close()

	var files []FileNode
	for rows.Next() {
		var f FileNode
		if err := rows.Scan(&f.FilePath); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, f)
	}
	return files, nil
}

// GraphData contains both call edges and dependency edges for a repo.
type GraphData struct {
	CallEdges []domain.CallEdge `json:"call_edges"`
	DepEdges  []domain.DepEdge  `json:"dep_edges"`
}

// GetGraph returns the call edges and dependency edges for a repo.
func (s *Service) GetGraph(ctx context.Context, repoID string) (*GraphData, error) {
	graph := &GraphData{
		CallEdges: []domain.CallEdge{},
		DepEdges:  []domain.DepEdge{},
	}

	// Query call edges
	callRows, err := s.db.Query(ctx,
		`SELECT id, repo_id, caller_id, callee_id, file_path, line FROM call_edges WHERE repo_id = $1 LIMIT 500`,
		repoID)
	if err != nil {
		return nil, fmt.Errorf("query call edges: %w", err)
	}
	defer callRows.Close()
	for callRows.Next() {
		var e domain.CallEdge
		if err := callRows.Scan(&e.ID, &e.RepoID, &e.CallerID, &e.CalleeID, &e.FilePath, &e.Line); err != nil {
			return nil, fmt.Errorf("scan call edge: %w", err)
		}
		graph.CallEdges = append(graph.CallEdges, e)
	}

	// Query dep edges
	depRows, err := s.db.Query(ctx,
		`SELECT id, repo_id, source_id, target_id, dep_type FROM dep_edges WHERE repo_id = $1 LIMIT 500`,
		repoID)
	if err != nil {
		return nil, fmt.Errorf("query dep edges: %w", err)
	}
	defer depRows.Close()
	for depRows.Next() {
		var e domain.DepEdge
		if err := depRows.Scan(&e.ID, &e.RepoID, &e.SourceID, &e.TargetID, &e.DepType); err != nil {
			return nil, fmt.Errorf("scan dep edge: %w", err)
		}
		graph.DepEdges = append(graph.DepEdges, e)
	}

	return graph, nil
}
