package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

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
	t, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *TaskHandler) List(c *gin.Context) {
	repoID := c.Query("repo_id")
	tasks, err := h.svc.List(c.Request.Context(), repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
