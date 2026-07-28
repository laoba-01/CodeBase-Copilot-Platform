package vectorstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/domain"
)

type SearchResult struct {
	Node  *domain.IndexNode
	Score float64
}

type Searcher struct {
	db *pgxpool.Pool
}

func NewSearcher(db *pgxpool.Pool) *Searcher {
	return &Searcher{db: db}
}

// SemanticSearch finds top-k similar nodes by cosine distance.
func (s *Searcher) SemanticSearch(ctx context.Context, repoID string, queryVec []float32, topK int) ([]SearchResult, error) {
	vecStr := vectorToString(queryVec)

	// Ensure ivfflat index is created first time
	s.db.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_nodes_embedding ON index_nodes USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)`)

	rows, err := s.db.Query(ctx, `
		SELECT id, repo_id, type, name, signature, code, file_path,
		       start_line, end_line, summary, language, package,
		       1 - (embedding <=> $1::vector) AS similarity
		FROM index_nodes
		WHERE repo_id = $2 AND embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT $3
	`, vecStr, repoID, topK)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

// GraphExpand expands search by ±1 hop call edges from seed nodes.
func (s *Searcher) GraphExpand(ctx context.Context, repoID string, seedIDs []string) ([]*domain.IndexNode, error) {
	if len(seedIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(seedIDs))
	args := make([]any, 0, len(seedIDs)+1)
	args = append(args, repoID)
	for i, id := range seedIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}

	// Same placeholders are reused for all three IN clauses, so args only needs one copy
	query := fmt.Sprintf(`
		SELECT DISTINCT n.id, n.repo_id, n.type, n.name, n.signature, n.code, n.file_path,
		       n.start_line, n.end_line, n.summary, n.language, n.package
		FROM index_nodes n
		JOIN call_edges e ON (n.id = e.callee_id OR n.id = e.caller_id)
		WHERE e.repo_id = $1 AND (e.caller_id IN (%s) OR e.callee_id IN (%s))
		AND n.id NOT IN (%s)
		LIMIT 50
	`, strings.Join(placeholders, ","), strings.Join(placeholders, ","), strings.Join(placeholders, ","))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("graph expand: %w", err)
	}
	defer rows.Close()

	var nodes []*domain.IndexNode
	for rows.Next() {
		var n domain.IndexNode
		if err := rows.Scan(&n.ID, &n.RepoID, &n.Type, &n.Name, &n.Signature, &n.Code,
			&n.FilePath, &n.StartLine, &n.EndLine, &n.Summary, &n.Language, &n.Package); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, &n)
	}
	return nodes, nil
}

func scanSearchResults(rows pgx.Rows) ([]SearchResult, error) {
	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var n domain.IndexNode
		if err := rows.Scan(&n.ID, &n.RepoID, &n.Type, &n.Name, &n.Signature, &n.Code,
			&n.FilePath, &n.StartLine, &n.EndLine, &n.Summary, &n.Language, &n.Package,
			&r.Score); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		r.Node = &n
		results = append(results, r)
	}
	return results, nil
}
