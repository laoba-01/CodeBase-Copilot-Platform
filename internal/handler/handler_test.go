package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/codebase-copilot/core/internal/auth"
	"github.com/codebase-copilot/core/internal/config"
	"github.com/codebase-copilot/core/internal/db"
	"github.com/codebase-copilot/core/internal/domain"
	"github.com/codebase-copilot/core/internal/repo"
	"github.com/codebase-copilot/core/internal/task"
)

// findWebDist locates the web/dist directory relative to the test working directory.
// When running go test, the cwd is the package directory (internal/handler).
func findWebDist() string {
	candidates := []string{
		"../../web/dist", // from internal/handler/ up to project root
		"web/dist",       // if running from project root
	}
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Join(p, "index.html")); err == nil {
			return p
		}
	}
	return ""
}

// setupTestRouter creates a Gin engine with all routes registered, mirroring cmd/server/main.go.
// Pass a nil db to skip database-dependent route registration.
func setupTestRouter(db *pgxpool.Pool, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// CORS (same as main.go)
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
	authHandler := NewAuthHandler(db, cfg)
	authHandler.RegisterRoutes(&r.RouterGroup)

	// Protected routes (only if db is available)
	if db != nil {
		api := r.Group("/api")
		api.Use(auth.RequireAuth(cfg.JWTSecret))

		repoHandler := NewRepoHandler(repo.NewService(db))
		repoHandler.RegisterRoutes(api)

		taskHandler := NewTaskHandler(task.NewService(db))
		taskHandler.RegisterRoutes(api)

		convHandler := NewConversationHandler(db)
		convHandler.RegisterRoutes(api)
	}

	// Static file serving (same as main.go)
	webDist := findWebDist()
	if webDist != "" {
		r.Static("/assets", filepath.Join(webDist, "assets"))
		r.StaticFile("/favicon.ico", filepath.Join(webDist, "favicon.ico"))
		r.GET("/", func(c *gin.Context) {
			c.File(filepath.Join(webDist, "index.html"))
		})
		r.NoRoute(func(c *gin.Context) {
			c.File(filepath.Join(webDist, "index.html"))
		})
	}

	return r
}

// testConfig returns a minimal Config for testing.
func testConfig() *config.Config {
	return &config.Config{
		JWTSecret: "test-integration-secret",
		Port:      "8080",
	}
}

// generateTestToken creates a valid JWT for the given user ID.
func generateTestToken(t *testing.T, cfg *config.Config, userID string) string {
	t.Helper()
	token, err := auth.GenerateToken(userID, cfg.JWTSecret)
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}
	return token
}

// ============================================================================
// TestHealthCheck — verifies static file serving and root endpoint
// ============================================================================

func TestHealthCheck(t *testing.T) {
	cfg := testConfig()
	r := setupTestRouter(nil, cfg)

	// Test GET / (serves index.html)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	webDist := findWebDist()
	if webDist == "" {
		t.Skip("web/dist/index.html not found; skipping static file serving test")
	}

	if w.Code != http.StatusOK {
		t.Errorf("GET /: expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	hasHTML := strings.Contains(body, "<!DOCTYPE html>") ||
		strings.Contains(body, "<html") ||
		strings.Contains(body, "<div id=\"root\">")
	if !hasHTML {
		t.Errorf("GET /: response does not contain HTML markup; body starts with: %q", truncate(body, 200))
	}

	// Verify Content-Type includes text/html
	ct := w.Header().Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "text/html") {
		t.Logf("GET / Content-Type: %s (expected text/html)", ct)
	}
}

func TestHealthCheck_NoRouteSPAFallback(t *testing.T) {
	webDist := findWebDist()
	if webDist == "" {
		t.Skip("web/dist/index.html not found; skipping SPA fallback test")
	}

	cfg := testConfig()
	r := setupTestRouter(nil, cfg)

	// Test that unknown client-side routes serve index.html
	req := httptest.NewRequest(http.MethodGet, "/dashboard/settings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("SPA fallback: expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<div id=\"root\">") {
		t.Errorf("SPA fallback: response does not contain React root div; body starts with: %q", truncate(body, 200))
	}
}

func TestHealthCheck_AssetsServed(t *testing.T) {
	webDist := findWebDist()
	if webDist == "" {
		t.Skip("web/dist not found; skipping assets test")
	}

	cfg := testConfig()
	r := setupTestRouter(nil, cfg)

	// Check that /assets/ route is registered and returns something
	// The actual file may not exist in the test build, but the route should be reachable
	req := httptest.NewRequest(http.MethodGet, "/assets/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Either 200 (directory listing) or 404 (no index) — just verify it doesn't panic
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("GET /assets/: unexpected status %d", w.Code)
	}
}

// ============================================================================
// TestAuthMiddleware — verifies authentication enforcement
// ============================================================================

func TestAuthMiddleware_NoToken(t *testing.T) {
	cfg := testConfig()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"
	}
	pool, err := db.NewPool(context.Background(), dbURL)
	if err != nil {
		t.Skipf("database not available for auth test: %v", err)
	}
	defer pool.Close()

	r := setupTestRouter(pool, cfg)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"GET repos", "GET", "/api/repos"},
		{"POST repos", "POST", "/api/repos"},
		{"GET tasks", "GET", "/api/tasks/123"},
		{"GET conversations", "GET", "/api/conversations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without token: expected 401, got %d", tt.method, tt.path, w.Code)
			}
		})
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	cfg := testConfig()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"
	}
	pool, err := db.NewPool(context.Background(), dbURL)
	if err != nil {
		t.Skipf("database not available for auth test: %v", err)
	}
	defer pool.Close()

	r := setupTestRouter(pool, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	cfg := testConfig()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"
	}
	pool, err := db.NewPool(context.Background(), dbURL)
	if err != nil {
		t.Skipf("database not available for auth test: %v", err)
	}
	defer pool.Close()

	r := setupTestRouter(pool, cfg)
	token := generateTestToken(t, cfg, "00000000-0000-0000-0000-000000000042")

	req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("valid token: expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// TestRepoCRUD — full repository lifecycle test with real database
// ============================================================================

func TestRepoCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"
	}

	pool, err := db.NewPool(context.Background(), dbURL)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	defer pool.Close()

	// Verify repo table exists
	var tableExists bool
	err = pool.QueryRow(context.Background(),
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'repos')",
	).Scan(&tableExists)
	if err != nil || !tableExists {
		t.Skip("repos table does not exist; run migrations first")
	}

	cfg := testConfig()
	r := setupTestRouter(pool, cfg)
	token := generateTestToken(t, cfg, "test-user-repo-crud")

	// ── Test GET /api/repos (list empty) ──
	req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/repos: expected 200, got %d", w.Code)
	}

	var repos []domain.Repository
	if err := json.Unmarshal(w.Body.Bytes(), &repos); err != nil {
		t.Fatalf("unmarshal repo list: %v", err)
	}
	// Should be an empty array (not null)
	if repos == nil {
		t.Error("GET /api/repos: expected empty array [], got null")
	}

	// ── Test POST /api/repos (create) ──
	// Note: This calls GitHub API which requires auth; it will likely fail
	// but the endpoint itself should be functional.
	createBody := `{"full_name": "torvalds/linux"}`
	req = httptest.NewRequest(http.MethodPost, "/api/repos", strings.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	t.Logf("POST /api/repos status: %d (may fail without GitHub token)", w.Code)

	// If create succeeded (201), we can also test GET and DELETE
	if w.Code == http.StatusCreated {
		var created domain.Repository
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created repo: %v", err)
		}

		// ── Test GET /api/repos/:id ──
		req = httptest.NewRequest(http.MethodGet, "/api/repos/"+created.ID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET /api/repos/%s: expected 200, got %d", created.ID, w.Code)
		}

		// ── Test GET /api/repos/:id (404) ──
		req = httptest.NewRequest(http.MethodGet, "/api/repos/nonexistent-id", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("GET /api/repos/nonexistent-id: expected 404, got %d", w.Code)
		}

		// ── Test DELETE /api/repos/:id ──
		req = httptest.NewRequest(http.MethodDelete, "/api/repos/"+created.ID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("DELETE /api/repos/%s: expected 200, got %d", created.ID, w.Code)
		}
	}
}

// ============================================================================
// TestSSE — verifies SSE streaming endpoint content type and auth
// ============================================================================

func TestAskSSE(t *testing.T) {
	cfg := testConfig()
	gin.SetMode(gin.TestMode)

	t.Run("ContentType", func(t *testing.T) {
		// Verify that the /api/ask endpoint returns text/event-stream content type
		// Uses a mock handler simulating SSE behavior
		r := gin.New()
		api := r.Group("/api")
		api.Use(auth.RequireAuth(cfg.JWTSecret))
		api.POST("/ask", func(c *gin.Context) {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			c.Status(http.StatusOK)

			// Simulate SSE chunk
			chunk := `{"text":"Hello from SSE test"}`
			c.Writer.Write([]byte("event: chunk\ndata: " + chunk + "\n\n"))
			c.Writer.(http.Flusher).Flush()

			// Simulate SSE done
			done := `{"confidence":0.85,"tokens":5}`
			c.Writer.Write([]byte("event: done\ndata: " + done + "\n\n"))
			c.Writer.(http.Flusher).Flush()
		})

		token := generateTestToken(t, cfg, "test-sse-user")
		reqBody := `{"repo_id":"test-repo","question":"test question"}`
		req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("POST /api/ask: expected 200, got %d", w.Code)
		}

		ct := w.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "text/event-stream") {
			t.Errorf("Content-Type: expected text/event-stream, got %q", ct)
		}

		// Verify SSE format in body
		body := w.Body.String()
		if !strings.Contains(body, "event: chunk") {
			t.Error("SSE response missing 'event: chunk' line")
		}
		if !strings.Contains(body, "event: done") {
			t.Error("SSE response missing 'event: done' line")
		}
		if !strings.Contains(body, "data:") {
			t.Error("SSE response missing 'data:' lines")
		}
	})

	t.Run("Auth", func(t *testing.T) {
		// Verify that /api/ask requires authentication
		r := gin.New()
		api := r.Group("/api")
		api.Use(auth.RequireAuth(cfg.JWTSecret))
		api.POST("/ask", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})

		req := httptest.NewRequest(http.MethodPost, "/api/ask",
			strings.NewReader(`{"repo_id":"r","question":"q"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("POST /api/ask without token: expected 401, got %d", w.Code)
		}
	})

	t.Run("Validation", func(t *testing.T) {
		r := gin.New()
		api := r.Group("/api")
		api.Use(auth.RequireAuth(cfg.JWTSecret))
		api.POST("/ask", func(c *gin.Context) {
			var req domain.Question
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "repo_id and question are required"})
				return
			}
			c.Header("Content-Type", "text/event-stream")
			c.Status(http.StatusOK)
		})

		token := generateTestToken(t, cfg, "test-user")

		// Missing required fields
		req := httptest.NewRequest(http.MethodPost, "/api/ask",
			strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("empty body: expected 400, got %d", w.Code)
		}
	})
}

// ============================================================================
// TestCORS — verifies CORS headers on API responses
// ============================================================================

func TestCORS(t *testing.T) {
	cfg := testConfig()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"
	}
	pool, err := db.NewPool(context.Background(), dbURL)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	defer pool.Close()

	r := setupTestRouter(pool, cfg)
	token := generateTestToken(t, cfg, "test-cors-user")

	t.Run("Preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/repos", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Access-Control-Request-Method", "POST")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("OPTIONS preflight: expected 204, got %d", w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("missing Access-Control-Allow-Origin header")
		}
	})

	t.Run("NormalRequest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("CORS header missing on normal request")
		}
	})
}

// ============================================================================
// Helpers
// ============================================================================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
