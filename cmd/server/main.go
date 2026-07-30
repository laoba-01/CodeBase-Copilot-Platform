package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codebase-copilot/core/internal/auth"
	"github.com/codebase-copilot/core/internal/config"
	"github.com/codebase-copilot/core/internal/db"
	"github.com/codebase-copilot/core/internal/embedding"
	"github.com/codebase-copilot/core/internal/handler"
	"github.com/codebase-copilot/core/internal/indexer"
	"github.com/codebase-copilot/core/internal/qa"
	"github.com/codebase-copilot/core/internal/repo"
	"github.com/codebase-copilot/core/internal/task"
	"github.com/codebase-copilot/core/internal/vectorstore"
)

func main() {
	cfg := config.Load()

	// Database
	pool, err := db.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// Run migrations
	if err := db.RunMigrations(context.Background(), pool, "/migrations"); err != nil {
		log.Printf("WARNING: migrations: %v", err)
	}

	// Recover repos stuck in transient states from a previous crash
	if err := db.RecoverStuckRepos(context.Background(), pool); err != nil {
		log.Printf("WARNING: stuck repo recovery: %v", err)
	}

	// Embedding client
	embClient, err := embedding.NewClient(context.Background(), cfg.EmbeddingAddr)
	if err != nil {
		log.Printf("WARNING: embedding service not available: %v", err)
		// Don't fatal — allow starting without embedding for development
	} else {
		defer embClient.Close()
	}

	// Services
	authHandler := handler.NewAuthHandler(pool, cfg)
	repoSvc := repo.NewService(pool)
	repoHandler := handler.NewRepoHandler(repoSvc)
	taskSvc := task.NewService(pool)
	taskHandler := handler.NewTaskHandler(taskSvc)

	vstore := vectorstore.NewStore(pool)
	vsearcher := vectorstore.NewSearcher(pool)

	// Indexer (depends on embedding client)
	var indexSvc *indexer.Service
	if embClient != nil {
		indexSvc = indexer.NewService(pool, embClient, vstore, "/data/repos")
		// Wire task hooks so indexing progress is persisted
		indexSvc.SetTaskHooks(indexer.TaskHooks{
			Create: taskSvc.Enqueue,
			Done:   taskSvc.Complete,
			Fail:   taskSvc.Fail,
		})
		repoSvc.Indexer = indexSvc.IndexRepo
	}

	// QA: RAG service
	var askHandler *handler.AskHandler
	if embClient != nil {
		llmClient := qa.NewLLMClient(qa.LLMConfig{
			Provider: cfg.LLMProvider,
			APIKey:   cfg.LLMAPIKey,
			Model:    cfg.LLMModel,
		})
		ragSvc := qa.NewRAGService(embClient, vsearcher, llmClient)
		askHandler = handler.NewAskHandler(ragSvc, pool)
	}

	convHandler := handler.NewConversationHandler(pool)

	// Gin router
	r := gin.Default()

	// Global middleware: body size limit (1MB) and rate limiting
	r.Use(handler.BodyLimit(1 << 20)) // 1 MB max request body

	// CORS with configurable origin (never wildcard in production)
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", cfg.AllowedOrigin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Security headers
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	})

	// Public routes (Any = GET+POST for GitHub OAuth callback)
	r.Any("/auth/github/callback", authHandler.GitHubCallback)
	r.GET("/auth/dev-login", authHandler.DevLogin)

	// Protected routes with rate limiting
	api := r.Group("/api")
	api.Use(auth.RequireAuth(cfg.JWTSecret))
	api.Use(handler.RateLimit(100, 200)) // 100 req/s burst 200
	repoHandler.RegisterRoutes(api)
	taskHandler.RegisterRoutes(api)
	convHandler.RegisterRoutes(api)
	if askHandler != nil {
		// Stricter rate limit for the expensive LLM endpoint
		askGroup := api.Group("")
		askGroup.Use(handler.StrictRateLimit(5, 10)) // 5 req/s burst 10
		askHandler.RegisterRoutes(askGroup)
	}

	// Rate limit auth endpoints
	r.Use(handler.RateLimit(20, 40)) // 20 req/s burst 40 for auth routes

	// Health check with dependency verification
	r.GET("/health", func(c *gin.Context) {
		healthy := true
		checks := gin.H{}

		// Check DB
		if err := pool.Ping(c.Request.Context()); err != nil {
			checks["database"] = "unhealthy"
			healthy = false
		} else {
			checks["database"] = "ok"
		}

		// Check embedding
		if embClient != nil {
			checks["embedding"] = "ok"
		} else {
			checks["embedding"] = "unavailable"
		}

		status := "ok"
		httpStatus := http.StatusOK
		if !healthy {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":  status,
			"version": "0.1.0",
			"checks":  checks,
		})
	})

	// Static file serving for React frontend
	if _, err := os.Stat("web/dist"); err == nil {
		r.Static("/assets", "web/dist/assets")
		r.StaticFile("/favicon.ico", "web/dist/favicon.ico")
		r.GET("/", func(c *gin.Context) {
			c.File("web/dist/index.html")
		})
		// SPA fallback: serve index.html for client-side routing
		r.NoRoute(func(c *gin.Context) {
			c.File("web/dist/index.html")
		})
	} else {
		log.Printf("WARNING: web/dist not found; frontend not served")
	}

	// Graceful shutdown with http.Server (with timeouts)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Long timeout for SSE streaming
		IdleTimeout:  60 * time.Second,
	}

	// Signal handler for graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("server starting on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
	log.Println("server stopped")
}
