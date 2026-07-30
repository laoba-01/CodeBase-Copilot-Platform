package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

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
	offset := 0
	limit := 50
	repos, err := h.svc.List(c.Request.Context(), userID, offset, limit)
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
	userID := auth.GetUserID(c)
	r, err := h.svc.Get(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *RepoHandler) Delete(c *gin.Context) {
	userID := auth.GetUserID(c)
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *RepoHandler) Files(c *gin.Context) {
	repoID := c.Param("id")
	userID := auth.GetUserID(c)

	// Verify repo ownership
	if _, err := h.svc.Get(c.Request.Context(), repoID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	files, err := h.svc.GetFiles(c.Request.Context(), repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if files == nil {
		files = []repo.FileNode{}
	}
	c.JSON(http.StatusOK, files)
}

func (h *RepoHandler) Graph(c *gin.Context) {
	repoID := c.Param("id")
	userID := auth.GetUserID(c)

	// Verify repo ownership
	if _, err := h.svc.Get(c.Request.Context(), repoID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	graph, err := h.svc.GetGraph(c.Request.Context(), repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, graph)
}

func (h *RepoHandler) Reindex(c *gin.Context) {
	repoID := c.Param("id")
	userID := auth.GetUserID(c)

	// Verify repo ownership and get repo
	r, err := h.svc.Get(c.Request.Context(), repoID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Trigger reindex
	if err := h.svc.Reindex(c.Request.Context(), r); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "reindexing", "repo_id": repoID})
}

type searchReq struct {
	Query string `json:"query" binding:"required"`
}

func (h *RepoHandler) Search(c *gin.Context) {
	repoID := c.Param("id")
	userID := auth.GetUserID(c)

	// Verify repo ownership
	if _, err := h.svc.Get(c.Request.Context(), repoID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	var req searchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	results, err := h.svc.SearchCode(c.Request.Context(), repoID, req.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

func (h *RepoHandler) FileContent(c *gin.Context) {
	repoID := c.Param("id")
	userID := auth.GetUserID(c)
	filePath := c.Query("path")
	if filePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query param is required"})
		return
	}

	// Verify repo ownership
	if _, err := h.svc.Get(c.Request.Context(), repoID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	nodes, err := h.svc.GetFileNodes(c.Request.Context(), repoID, filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if nodes == nil {
		nodes = []repo.FileContentNode{}
	}
	c.JSON(http.StatusOK, nodes)
}

func (h *RepoHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/repos", h.Create)
	r.GET("/repos", h.List)
	r.GET("/repos/:id", h.Get)
	r.DELETE("/repos/:id", h.Delete)
	r.GET("/repos/:id/files", h.Files)
	r.GET("/repos/:id/file-content", h.FileContent)
	r.GET("/repos/:id/graph", h.Graph)
	r.POST("/repos/:id/reindex", h.Reindex)
	r.POST("/repos/:id/search", h.Search)
}
