package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/auth"
	"github.com/codebase-copilot/core/internal/domain"
	"github.com/codebase-copilot/core/internal/qa"
)

type AskHandler struct {
	rag *qa.RAGService
	db  *pgxpool.Pool
}

func NewAskHandler(rag *qa.RAGService, db *pgxpool.Pool) *AskHandler {
	return &AskHandler{rag: rag, db: db}
}

func (h *AskHandler) Ask(c *gin.Context) {
	var req domain.Question
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repo_id and question are required"})
		return
	}
	userID := auth.GetUserID(c)

	// Create or get conversation
	convID := req.ConversationID
	if convID == "" {
		convID = uuid.New().String()
		title := req.Question
		if len(title) > 100 {
			title = title[:100]
		}
		h.db.Exec(c.Request.Context(),
			`INSERT INTO conversations (id, user_id, repo_id, title) VALUES ($1,$2,$3,$4)`,
			convID, userID, req.RepoID, title)
	}

	// Save user message
	h.db.Exec(c.Request.Context(),
		`INSERT INTO messages (id, conv_id, role, content) VALUES ($1,$2,'user',$3)`,
		uuid.New().String(), convID, req.Question)

	// Load history
	history := h.loadHistory(c.Request.Context(), convID)

	// Run RAG pipeline (streams directly to ResponseWriter)
	if err := h.rag.Ask(c.Request.Context(), req.RepoID, req.Question, history, c.Writer); err != nil {
		// Error is already written to SSE stream, but log it
		_ = err
	}
}

func (h *AskHandler) loadHistory(ctx context.Context, convID string) []domain.Message {
	rows, err := h.db.Query(ctx,
		`SELECT id, conv_id, role, content FROM messages WHERE conv_id = $1 ORDER BY created_at ASC LIMIT 20`,
		convID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var msgs []domain.Message
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.ID, &m.ConvID, &m.Role, &m.Content); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func (h *AskHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/ask", h.Ask)
}
