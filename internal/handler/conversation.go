package handler

import (
	"encoding/json"
	"fmt"
	"log"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	defer rows.Close()

	var convs []domain.Conversation
	for rows.Next() {
		var conv domain.Conversation
		var createdAt any
		if err := rows.Scan(&conv.ID, &conv.UserID, &conv.RepoID, &conv.Title, &createdAt); err != nil {
			log.Printf("ERROR: scan conversation: %v", err)
			continue
		}
		conv.CreatedAt = fmt.Sprintf("%v", createdAt)
		convs = append(convs, conv)
	}
	if err := rows.Err(); err != nil {
		log.Printf("ERROR: iterate conversations: %v", err)
	}
	if convs == nil {
		convs = []domain.Conversation{}
	}
	c.JSON(http.StatusOK, convs)
}

func (h *ConversationHandler) Get(c *gin.Context) {
	convID := c.Param("id")
	userID := auth.GetUserID(c)

	// Verify conversation ownership
	var ownerID string
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT user_id FROM conversations WHERE id = $1`, convID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}
	if ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, conv_id, role, content, citations, tokens, created_at FROM messages WHERE conv_id = $1 ORDER BY created_at ASC`,
		convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	defer rows.Close()

	var msgs []domain.Message
	for rows.Next() {
		var m domain.Message
		var citationsJSON []byte
		var createdAt any
		if err := rows.Scan(&m.ID, &m.ConvID, &m.Role, &m.Content, &citationsJSON, &m.Tokens, &createdAt); err != nil {
			log.Printf("ERROR: scan message: %v", err)
			continue
		}
		if len(citationsJSON) > 0 {
			if err := json.Unmarshal(citationsJSON, &m.Citations); err != nil {
				log.Printf("ERROR: unmarshal citations: %v", err)
			}
		}
		m.CreatedAt = fmt.Sprintf("%v", createdAt)
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		log.Printf("ERROR: iterate messages: %v", err)
	}
	c.JSON(http.StatusOK, msgs)
}

func (h *ConversationHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/conversations", h.List)
	r.GET("/conversations/:id", h.Get)
}
