package vectorstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/domain"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Insert(ctx context.Context, node *domain.IndexNode) error {
	var vecArg any
	if len(node.Embedding) == 0 {
		vecArg = nil
	} else {
		vecArg = vectorToString(node.Embedding)
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO index_nodes (id, repo_id, type, name, signature, code, file_path,
			start_line, end_line, summary, embedding, language, package, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::vector, $12, $13, '{}')
	`, node.ID, node.RepoID, node.Type, node.Name, node.Signature, node.Code,
		node.FilePath, node.StartLine, node.EndLine, node.Summary,
		vecArg, node.Language, node.Package)
	return err
}

func (s *Store) BatchInsert(ctx context.Context, nodes []*domain.IndexNode) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, node := range nodes {
		var vecArg any
		if len(node.Embedding) == 0 {
			vecArg = nil
		} else {
			vecArg = vectorToString(node.Embedding)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO index_nodes (id, repo_id, type, name, signature, code, file_path,
				start_line, end_line, summary, embedding, language, package)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::vector, $12, $13)
		`, node.ID, node.RepoID, node.Type, node.Name, node.Signature, node.Code,
			node.FilePath, node.StartLine, node.EndLine, node.Summary,
			vecArg, node.Language, node.Package)
		if err != nil {
			return fmt.Errorf("insert node %s: %w", node.ID, err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteByRepo(ctx context.Context, repoID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM index_nodes WHERE repo_id = $1`, repoID)
	return err
}

func vectorToString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	s := fmt.Sprintf("[%f", v[0])
	for i := 1; i < len(v); i++ {
		s += fmt.Sprintf(",%f", v[i])
	}
	s += "]"
	return s
}
