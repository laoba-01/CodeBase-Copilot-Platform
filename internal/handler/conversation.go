package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/auth"
	"github.com/codebase-copilot/core/internal/domain"
)

type ConversationHandler struct {
	db *pgxpool.Pool
}

func NewConversationHandler(db *pgxpool.Pool) *ConversationHandler {
	return &ConversationHandler{db: db}
}

func (h *ConversationHandler) List(c *gin.Context) {
	userID := auth.GetUserID(c)
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, user_id, repo_id, title, created_at FROM conversations WHERE user_id = $1 ORDER BY created_at DESC`,
		userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var convs []domain.Conversation
	for rows.Next() {
		var conv domain.Conversation
		var createdAt any
		if err := rows.Scan(&conv.ID, &conv.UserID, &conv.RepoID, &conv.Title, &createdAt); err != nil {
			continue
		}
		conv.CreatedAt = fmt.Sprintf("%v", createdAt)
		convs = append(convs, conv)
	}
	c.JSON(http.StatusOK, convs)
}

func (h *ConversationHandler) Get(c *gin.Context) {
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, conv_id, role, content, citations, tokens, created_at FROM messages WHERE conv_id = $1 ORDER BY created_at ASC`,
		c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var msgs []domain.Message
	for rows.Next() {
		var m domain.Message
		var citationsJSON []byte
		var createdAt any
		if err := rows.Scan(&m.ID, &m.ConvID, &m.Role, &m.Content, &citationsJSON, &m.Tokens, &createdAt); err != nil {
			continue
		}
		if len(citationsJSON) > 0 {
			json.Unmarshal(citationsJSON, &m.Citations)
		}
		m.CreatedAt = fmt.Sprintf("%v", createdAt)
		msgs = append(msgs, m)
	}
	c.JSON(http.StatusOK, msgs)
}

func (h *ConversationHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/conversations", h.List)
	r.GET("/conversations/:id", h.Get)
}
