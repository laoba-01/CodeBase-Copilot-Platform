package indexer

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/domain"
	"github.com/codebase-copilot/core/internal/embedding"
	"github.com/codebase-copilot/core/internal/vectorstore"
)

// Service orchestrates code indexing: clone → parse → embed → store.
type Service struct {
	db         *pgxpool.Pool
	emb        *embedding.Client
	store      *vectorstore.Store
	dataDir    string // e.g. /data/repos
	taskCreate func(ctx context.Context, repoID string, taskType domain.TaskType) (*domain.Task, error)
	taskDone   func(ctx context.Context, taskID, result string) error
	taskFail   func(ctx context.Context, taskID, errMsg string) error
}

// TaskHooks provides callbacks for persisting task records during indexing.
type TaskHooks struct {
	Create func(ctx context.Context, repoID string, taskType domain.TaskType) (*domain.Task, error)
	Done   func(ctx context.Context, taskID, result string) error
	Fail   func(ctx context.Context, taskID, errMsg string) error
}

// NewService creates a new indexing service.
func NewService(db *pgxpool.Pool, emb *embedding.Client, store *vectorstore.Store, dataDir string) *Service {
	return &Service{db: db, emb: emb, store: store, dataDir: dataDir}
}

// SetTaskHooks wires task recording callbacks into the indexer.
func (s *Service) SetTaskHooks(hooks TaskHooks) {
	s.taskCreate = hooks.Create
	s.taskDone = hooks.Done
	s.taskFail = hooks.Fail
}

// IndexRepo clones (or pulls) a repository, parses its Go code, generates
// embeddings for function-level nodes, and persists everything to the database.
func (s *Service) IndexRepo(ctx context.Context, repo *domain.Repository) error {
	repoPath := fmt.Sprintf("%s/%s", s.dataDir, repo.ID)

	// Create task record for tracking
	var taskID string
	if s.taskCreate != nil {
		t, err := s.taskCreate(ctx, repo.ID, domain.TaskTypeIndex)
		if err == nil {
			taskID = t.ID
		}
	}

	// Step 1: Clone repo if not already cloned
	var err error
	if err = s.ensureCloned(ctx, repo, repoPath); err != nil {
		s.recordTaskFail(ctx, taskID, err.Error())
		return fmt.Errorf("clone: %w", err)
	}

	// Update status to indexing
	s.db.Exec(ctx, `UPDATE repos SET status = 'indexing' WHERE id = $1`, repo.ID)

	// Step 2: Parse code → extract nodes, call edges, dep edges
	lang := detectLanguage(repoPath)
	var nodes []*domain.IndexNode
	var calls []*domain.CallEdge
	var deps []*domain.DepEdge
	if lang == "go" {
		nodes, calls, deps, err = ParseGoRepo(repoPath, repo.ID)
	} else {
		nodes, calls, deps, err = ParseUniversal(repoPath, repo.ID)
	}
	if err != nil {
		s.recordTaskFail(ctx, taskID, err.Error())
		return fmt.Errorf("parse repo (%s): %w", lang, err)
	}

	// Step 3: Generate embeddings for all function-level nodes
	var funcNodes []*domain.IndexNode
	embTexts := make([]string, 0)
	for _, n := range nodes {
		if n.Type == domain.NodeTypeFunction || n.Type == domain.NodeTypeMethod {
			embText := n.Signature + "\n" + n.Code
			embTexts = append(embTexts, embText)
			funcNodes = append(funcNodes, n)
		}
	}

	if len(embTexts) > 0 {
		// Batch embed in groups of 4
		batchSize := 4
		for i := 0; i < len(embTexts); i += batchSize {
			end := i + batchSize
			if end > len(embTexts) {
				end = len(embTexts)
			}
			batch := embTexts[i:end]
			vectors, err := s.emb.Embed(ctx, batch)
			if err != nil {
				s.recordTaskFail(ctx, taskID, err.Error())
				return fmt.Errorf("embed batch %d: %w", i/batchSize, err)
			}
			for j, v := range vectors {
				funcNodes[i+j].Embedding = v
			}
		}
	}

	// Step 4: Clear existing index and edges, then insert new
	if err := deleteEdgesByRepo(ctx, s.db, repo.ID); err != nil {
		s.recordTaskFail(ctx, taskID, err.Error())
		return fmt.Errorf("clear old edges: %w", err)
	}
	if err := s.store.DeleteByRepo(ctx, repo.ID); err != nil {
		s.recordTaskFail(ctx, taskID, err.Error())
		return fmt.Errorf("clear old index: %w", err)
	}
	if len(nodes) > 0 {
		if err := s.store.BatchInsert(ctx, nodes); err != nil {
			s.recordTaskFail(ctx, taskID, err.Error())
			return fmt.Errorf("insert nodes: %w", err)
		}
	}

	// Step 5: Insert call edges and dep edges in a transaction
	if err := insertEdges(ctx, s.db, repo.ID, calls, deps); err != nil {
		s.recordTaskFail(ctx, taskID, err.Error())
		return fmt.Errorf("insert edges: %w", err)
	}

	// Step 6: Mark ready
	s.db.Exec(ctx, `UPDATE repos SET status = 'ready', indexed_at = now() WHERE id = $1`, repo.ID)

	// Complete task
	if taskID != "" && s.taskDone != nil {
		s.taskDone(ctx, taskID, fmt.Sprintf(`{"nodes":%d,"calls":%d,"deps":%d}`, len(nodes), len(calls), len(deps)))
	}

	return nil
}

// recordTaskFail marks a task as failed if one was created.
func (s *Service) recordTaskFail(ctx context.Context, taskID, errMsg string) {
	if taskID == "" || s.taskFail == nil {
		return
	}
	s.taskFail(ctx, taskID, errMsg)
}

// ensureCloned clones the repo if it doesn't exist locally, or pulls latest.
// It respects HTTP_PROXY/HTTPS_PROXY environment variables for environments
// that need a proxy to reach GitHub (e.g. Docker behind a host proxy).
// SSL verification is disabled for environments with custom CA certificates.
func (s *Service) ensureCloned(ctx context.Context, repo *domain.Repository, path string) error {
	gitArgs := []string{
		"-c", "http.sslVerify=false",
	}

	// Only set proxy override if explicit env var is present;
	// otherwise inherit from the environment (global git config or env vars).
	if proxy := os.Getenv("GIT_PROXY"); proxy != "" {
		gitArgs = append(gitArgs, "-c", "http.proxy="+proxy)
		gitArgs = append(gitArgs, "-c", "https.proxy="+proxy)
	}

	// Check if already cloned
	if _, err := os.Stat(path); err == nil {
		// Pull latest
		args := append([]string{}, gitArgs...)
		args = append(args, "-C", path, "pull", "origin", repo.DefaultBranch)
		cmd := exec.CommandContext(ctx, "git", args...)
		return cmd.Run()
	}

	// Clone
	args := append([]string{}, gitArgs...)
	args = append(args, "clone", "--depth", "1", "--branch", repo.DefaultBranch, repo.CloneURL, path)
	cmd := exec.CommandContext(ctx, "git", args...)
	return cmd.Run()
}

// deleteEdgesByRepo removes all call and dep edges for a repo (run before
// deleting index_nodes, since edges FK-reference nodes).
func deleteEdgesByRepo(ctx context.Context, db *pgxpool.Pool, repoID string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM call_edges WHERE repo_id = $1`, repoID); err != nil {
		return fmt.Errorf("delete call_edges: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dep_edges WHERE repo_id = $1`, repoID); err != nil {
		return fmt.Errorf("delete dep_edges: %w", err)
	}

	return tx.Commit(ctx)
}

// insertEdges inserts call edges and dep edges within a single transaction,
// returning the first error encountered.
func insertEdges(ctx context.Context, db *pgxpool.Pool, repoID string, calls []*domain.CallEdge, deps []*domain.DepEdge) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, e := range calls {
		if !isUUID(e.CalleeID) {
			continue
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO call_edges (id, repo_id, caller_id, callee_id, file_path, line) VALUES ($1,$2,$3,$4,$5,$6)`,
			e.ID, e.RepoID, e.CallerID, e.CalleeID, e.FilePath, e.Line)
		if err != nil {
			return fmt.Errorf("insert call edge %s: %w", e.ID, err)
		}
	}

	for _, e := range deps {
		// Skip edges with non-UUID target IDs (external deps like C includes, Go imports)
		if !isUUID(e.TargetID) {
			continue
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO dep_edges (id, repo_id, source_id, target_id, dep_type) VALUES ($1,$2,$3,$4,$5)`,
			e.ID, e.RepoID, e.SourceID, e.TargetID, e.DepType)
		if err != nil {
			return fmt.Errorf("insert dep edge %s: %w", e.ID, err)
		}
	}

	return tx.Commit(ctx)
}

// isUUID checks if a string looks like a valid UUID (36 chars, 4 hyphens).
func isUUID(s string) bool {
	return len(s) == 36 &&
		s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}
