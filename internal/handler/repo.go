package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/codebase-copilot/core/internal/auth"
	"github.com/codebase-copilot/core/internal/domain"
	"github.com/codebase-copilot/core/internal/repo"
)

type RepoHandler struct {
	svc *repo.Service
}

func NewRepoHandler(svc *repo.Service) *RepoHandler {
	return &RepoHandler{svc: svc}
}

type createRepoReq struct {
	FullName string `json:"full_name" binding:"required"` // owner/repo
}

func (h *RepoHandler) Create(c *gin.Context) {
	var req createRepoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "full_name is required"})
		return
	}
	userID := auth.GetUserID(c)
	r, err := h.svc.Create(c.Request.Context(), userID, req.FullName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}

func (h *RepoHandler) List(c *gin.Context) {
	userID := auth.GetUserID(c)
	repos, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if repos == nil {
		repos = []domain.Repository{}
	}
	c.JSON(http.StatusOK, repos)
}

func (h *RepoHandler) Get(c *gin.Context) {
	r, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *RepoHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *RepoHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/repos", h.Create)
	r.GET("/repos", h.List)
	r.GET("/repos/:id", h.Get)
	r.DELETE("/repos/:id", h.Delete)
}
