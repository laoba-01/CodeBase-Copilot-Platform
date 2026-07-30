package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/codebase-copilot/core/internal/auth"
	"github.com/codebase-copilot/core/internal/domain"
	"github.com/codebase-copilot/core/internal/task"
)

type TaskHandler struct {
	svc *task.Service
}

func NewTaskHandler(svc *task.Service) *TaskHandler {
	return &TaskHandler{svc: svc}
}

func (h *TaskHandler) Get(c *gin.Context) {
	userID := auth.GetUserID(c)
	t, err := h.svc.Get(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		} else if errors.Is(err, task.ErrAccessDenied) {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *TaskHandler) List(c *gin.Context) {
	userID := auth.GetUserID(c)
	repoID := c.Query("repo_id")
	tasks, err := h.svc.List(c.Request.Context(), repoID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if tasks == nil {
		tasks = []domain.Task{}
	}
	c.JSON(http.StatusOK, tasks)
}

func (h *TaskHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/tasks", h.List)
	r.GET("/tasks/:id", h.Get)
}
