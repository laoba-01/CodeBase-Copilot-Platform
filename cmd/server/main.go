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
	if err := db.RunMigrations(context.Background(), pool, "migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
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
	}
	_ = indexSvc // referenced for future background task workers

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

	// CORS
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Public routes
	authHandler.RegisterRoutes(&r.RouterGroup)

	// Protected routes
	api := r.Group("/api")
	api.Use(auth.RequireAuth(cfg.JWTSecret))
	repoHandler.RegisterRoutes(api)
	taskHandler.RegisterRoutes(api)
	convHandler.RegisterRoutes(api)
	if askHandler != nil {
		askHandler.RegisterRoutes(api)
	}

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

	// Graceful shutdown with http.Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
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
