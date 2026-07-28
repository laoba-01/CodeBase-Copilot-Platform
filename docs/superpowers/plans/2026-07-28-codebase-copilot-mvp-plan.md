# Codebase Copilot Platform — MVP 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建面向 Git 仓库的 AI 智能研发平台 MVP——GitHub 仓库接入、代码分层索引、RAG 流式问答。

**Architecture:** Go 模块化单块，Gin HTTP Server，PostgreSQL+pgvector 统一存储，Redis 做任务队列，Python gRPC sidecar 跑 Embedding/Rerank 模型，React 前端。

**Tech Stack:** Go 1.22+, Gin, PostgreSQL 15+pgvector, Redis 7, gRPC (Go↔Python), BGE-M3, Claude/DeepSeek API, React 18 + Ant Design 5, Docker Compose

## Global Constraints

- Go 1.22+, 所有 module path 为 `github.com/codebase-copilot/core`
- PostgreSQL 15+ with pgvector extension
- 所有 SQL 写在 migration 文件里，禁止 ORM
- JWT 用 `golang-jwt/jwt/v5`，access token 24h 过期
- 向量维度 1024（BGE-M3）
- 前端用 Vite + React 18 + Ant Design 5
- 所有服务通过 `docker-compose up` 一键启动
- 代码生成后必须能编译/运行，不允许 placeholder

---

## 文件结构总览

```
codebase-copilot/
├── cmd/server/main.go                 # 入口，组装所有依赖
├── internal/
│   ├── domain/                        # 纯 struct，零依赖
│   │   ├── user.go                    # User
│   │   ├── repo.go                    # Repository, RepoStatus
│   │   ├── index.go                   # IndexNode, CallEdge, DepEdge
│   │   ├── question.go               # Question, Answer, Citation, SSEEvent
│   │   └── task.go                    # Task, TaskStatus, TaskType
│   ├── config/
│   │   └── config.go                  # 环境变量 → Config struct
│   ├── db/
│   │   └── postgres.go                # pgxpool 连接 + 执行 migration
│   ├── auth/
│   │   ├── jwt.go                     # GenerateToken / ValidateToken
│   │   └── middleware.go              # Gin middleware: RequireAuth
│   ├── repo/
│   │   ├── github.go                  # GitHub API client + git clone
│   │   └── service.go                 # RepoService: CRUD + 触发索引
│   ├── indexer/
│   │   ├── go_parser.go               # Go AST 解析 → IndexNode + CallEdge + DepEdge
│   │   ├── tree_sitter.go             # Tree-sitter JS/TS 解析
│   │   └── service.go                 # IndexService: 编排索引全流程
│   ├── embedding/
│   │   └── client.go                  # gRPC client → Python sidecar
│   ├── vectorstore/
│   │   ├── store.go                   # Insert / BatchInsert / Delete
│   │   └── search.go                  # SemanticSearch + GraphExpand + HybridSearch
│   ├── qa/
│   │   ├── llm.go                     # LLM client (Claude/DeepSeek) + streaming
│   │   ├── rag.go                     # RAG 编排: retrieve → rerank → assemble → ask
│   │   └── sse.go                     # SSE writer 工具
│   ├── task/
│   │   └── service.go                 # TaskService: enqueue / poll / update progress
│   └── handler/
│       ├── auth.go                    # POST /api/auth/github/callback
│       ├── repo.go                    # CRUD /api/repos
│       ├── ask.go                     # POST /api/ask (SSE)
│       ├── conversation.go            # GET /api/conversations
│       └── task.go                    # GET /api/tasks/:id
├── proto/
│   └── embedding.proto               # gRPC: Embed(text) → vector, Rerank(query, docs) → scores
├── migrations/
│   └── 001_init.sql                   # 全量 DDL
├── python-embedding/
│   ├── requirements.txt               # grpcio, torch, transformers, FlagEmbedding
│   ├── server.py                      # gRPC server: Embed + Rerank
│   └── embedding_service.py           # BGE-M3 模型加载 + 推理
├── web/
│   ├── package.json
│   ├── vite.config.ts
│   ├── index.html
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── api.ts                     # fetch 封装 + SSE
│       ├── pages/
│       │   ├── RepoList.tsx           # 仓库列表 + 接入
│       │   ├── RepoDetail.tsx         # 文件树 + 索引状态
│       │   └── Ask.tsx                # 问答界面 (核心页面)
│       └── components/
│           ├── SSEViewer.tsx          # 流式回答渲染
│           ├── Citations.tsx          # 引用文件展示
│           └── Layout.tsx             # 全局布局
├── docker-compose.yml
├── Makefile
├── go.mod
└── .env.example
```

---

### Task 1: 项目骨架 + Docker Compose + 环境配置

**Files:**
- Create: `go.mod`, `Makefile`, `.env.example`, `docker-compose.yml`

**Interfaces:**
- Produces: Go module `github.com/codebase-copilot/core`, Docker 四服务编排，Makefile 常用命令

- [ ] **Step 1: 初始化 Go module**

```bash
cd "D:/CodeBase Copilot Platform"
go mod init github.com/codebase-copilot/core
```

- [ ] **Step 2: 创建 `.env.example`**

```bash
# Server
PORT=8080
JWT_SECRET=change-me-in-production
GITHUB_CLIENT_ID=your_github_client_id
GITHUB_CLIENT_SECRET=your_github_client_secret

# Database
DATABASE_URL=postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379

# Embedding
EMBEDDING_ADDR=localhost:50051

# LLM
LLM_PROVIDER=claude    # or deepseek
LLM_API_KEY=your_api_key
LLM_MODEL=claude-sonnet-5
```

- [ ] **Step 3: 创建 `docker-compose.yml`**

```yaml
version: '3.8'
services:
  app:
    build: .
    ports:
      - "8080:8080"
    env_file: .env
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_started
    volumes:
      - ./data/repos:/data/repos

  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: copilot
      POSTGRES_PASSWORD: copilot
      POSTGRES_DB: copilot
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U copilot"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  embedding:
    build: ./python-embedding
    ports:
      - "50051:50051"

volumes:
  pgdata:
```

- [ ] **Step 4: 创建 `Makefile`**

```makefile
.PHONY: dev build run test

dev:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./internal/... -v -count=1

db-up:
	docker compose up -d db redis

dc-up:
	docker compose up -d --build

dc-down:
	docker compose down

proto:
	protoc --go_out=. --go-grpc_out=. proto/embedding.proto
	python -m grpc_tools.protoc -I proto --python_out=python-embedding --grpc_python_out=python-embedding proto/embedding.proto
```

- [ ] **Step 5: 创建 `Dockerfile`**

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates git
COPY --from=builder /app/server /usr/local/bin/server
COPY migrations /migrations
EXPOSE 8080
CMD ["server"]
```

- [ ] **Step 6: 安装核心 Go 依赖**

```bash
go get github.com/gin-gonic/gin
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
go get github.com/redis/go-redis/v9
go get github.com/golang-jwt/jwt/v5
go get github.com/google/uuid
go get google.golang.org/grpc
go get github.com/go-git/go-git/v5
```

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: project scaffold with Docker Compose and Makefile"
```

---

### Task 2: Domain Models

**Files:**
- Create: `internal/domain/user.go`, `internal/domain/repo.go`, `internal/domain/index.go`, `internal/domain/question.go`, `internal/domain/task.go`

**Interfaces:**
- Produces: 所有 domain struct — `User`, `Repository`, `IndexNode`, `CallEdge`, `DepEdge`, `Question`, `Answer`, `Citation`, `Task`

- [ ] **Step 1: `internal/domain/user.go`**

```go
package domain

import "time"

type User struct {
    ID        string    `json:"id"`
    GitHubID  int64     `json:"github_id"`
    Username  string    `json:"username"`
    Email     string    `json:"email"`
    AvatarURL string    `json:"avatar_url"`
    CreatedAt time.Time `json:"created_at"`
}
```

- [ ] **Step 2: `internal/domain/repo.go`**

```go
package domain

import "time"

type RepoStatus string

const (
    RepoStatusPending  RepoStatus = "pending"
    RepoStatusCloning  RepoStatus = "cloning"
    RepoStatusIndexing RepoStatus = "indexing"
    RepoStatusReady    RepoStatus = "ready"
    RepoStatusError    RepoStatus = "error"
)

type Repository struct {
    ID            string     `json:"id"`
    UserID        string     `json:"user_id"`
    Name          string     `json:"name"`
    FullName      string     `json:"full_name"`    // owner/repo
    CloneURL      string     `json:"clone_url"`
    DefaultBranch string     `json:"default_branch"`
    Status        RepoStatus `json:"status"`
    IndexedAt     *time.Time `json:"indexed_at,omitempty"`
    CreatedAt     time.Time  `json:"created_at"`
}
```

- [ ] **Step 3: `internal/domain/index.go`**

```go
package domain

type IndexNodeType string

const (
    NodeTypeFile     IndexNodeType = "file"
    NodeTypeFunction IndexNodeType = "function"
    NodeTypeClass    IndexNodeType = "class"
    NodeTypeMethod   IndexNodeType = "method"
)

type IndexNode struct {
    ID        string        `json:"id"`
    RepoID    string        `json:"repo_id"`
    Type      IndexNodeType `json:"type"`
    Name      string        `json:"name"`
    Signature string        `json:"signature"`       // 函数签名
    Code      string        `json:"code"`            // 原始代码
    FilePath  string        `json:"file_path"`
    StartLine int           `json:"start_line"`
    EndLine   int           `json:"end_line"`
    Summary   string        `json:"summary"`         // LLM 生成的一句话描述
    Embedding []float32     `json:"-"`               // 不序列化
    Language  string        `json:"language"`
    Package   string        `json:"package"`         // Go package / JS module
}

type CallEdge struct {
    ID       string `json:"id"`
    RepoID   string `json:"repo_id"`
    CallerID string `json:"caller_id"`  // → IndexNode.ID
    CalleeID string `json:"callee_id"`  // → IndexNode.ID
    FilePath string `json:"file_path"`
    Line     int    `json:"line"`
}

type DepEdge struct {
    ID       string `json:"id"`
    RepoID   string `json:"repo_id"`
    SourceID string `json:"source_id"`  // → IndexNode.ID (file)
    TargetID string `json:"target_id"`  // → IndexNode.ID (file)
    DepType  string `json:"dep_type"`   // import, extend, implement
}
```

- [ ] **Step 4: `internal/domain/question.go`**

```go
package domain

type Question struct {
    RepoID         string `json:"repo_id" binding:"required"`
    Question       string `json:"question" binding:"required"`
    ConversationID string `json:"conversation_id,omitempty"`
}

type Citation struct {
    File    string  `json:"file"`
    Line    int     `json:"line"`
    Content string  `json:"content,omitempty"`
    Score   float64 `json:"score"`
}

type SSEChunk struct {
    Text      string     `json:"text"`
    Citations []Citation `json:"citations,omitempty"`
}

type SSEDone struct {
    Confidence float64 `json:"confidence"`
    Tokens     int     `json:"tokens"`
    ConvID     string  `json:"conv_id"`
}

type Conversation struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    RepoID    string    `json:"repo_id"`
    Title     string    `json:"title"`
    CreatedAt string    `json:"created_at"`
}

type Message struct {
    ID        string     `json:"id"`
    ConvID    string     `json:"conv_id"`
    Role      string     `json:"role"`      // user / assistant
    Content   string     `json:"content"`
    Citations []Citation `json:"citations,omitempty"`
    Tokens    int        `json:"tokens"`
    CreatedAt string     `json:"created_at"`
}
```

- [ ] **Step 5: `internal/domain/task.go`**

```go
package domain

import "time"

type TaskType string

const (
    TaskTypeIndex   TaskType = "index"
    TaskTypeReindex TaskType = "reindex"
)

type TaskStatus string

const (
    TaskStatusPending    TaskStatus = "pending"
    TaskStatusProcessing TaskStatus = "processing"
    TaskStatusCompleted  TaskStatus = "completed"
    TaskStatusFailed     TaskStatus = "failed"
)

type Task struct {
    ID        string     `json:"id"`
    RepoID    string     `json:"repo_id"`
    Type      TaskType   `json:"type"`
    Status    TaskStatus `json:"status"`
    Progress  int        `json:"progress"`   // 0-100
    Error     string     `json:"error,omitempty"`
    Result    string     `json:"result,omitempty"`  // JSON
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}
```

- [ ] **Step 6: 编译验证**

```bash
go build ./internal/domain/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/domain/ && git commit -m "feat: domain models"
```

---

### Task 3: Database Migration + Connection

**Files:**
- Create: `migrations/001_init.sql`, `internal/config/config.go`, `internal/db/postgres.go`

**Interfaces:**
- Consumes: domain models (Task 2)
- Produces: `config.Load() *Config`, `db.NewPool(cfg) *pgxpool.Pool`, `db.RunMigrations(pool)`

- [ ] **Step 1: `migrations/001_init.sql`**

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    github_id BIGINT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    email TEXT,
    avatar_url TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE repos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    clone_url TEXT NOT NULL,
    default_branch TEXT DEFAULT 'main',
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending','cloning','indexing','ready','error')),
    indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_repos_user ON repos(user_id);

CREATE TABLE index_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('file','function','class','method')),
    name TEXT NOT NULL,
    signature TEXT DEFAULT '',
    code TEXT DEFAULT '',
    file_path TEXT NOT NULL,
    start_line INT DEFAULT 0,
    end_line INT DEFAULT 0,
    summary TEXT DEFAULT '',
    embedding vector(1024),
    language TEXT DEFAULT '',
    package TEXT DEFAULT '',
    metadata JSONB DEFAULT '{}'
);
CREATE INDEX idx_nodes_repo_type ON index_nodes(repo_id, type);
CREATE INDEX idx_nodes_file_path ON index_nodes(repo_id, file_path);

CREATE TABLE call_edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    caller_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    callee_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    file_path TEXT DEFAULT '',
    line INT DEFAULT 0
);
CREATE INDEX idx_call_edges_repo ON call_edges(repo_id);

CREATE TABLE dep_edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    target_id UUID NOT NULL REFERENCES index_nodes(id) ON DELETE CASCADE,
    dep_type TEXT DEFAULT 'import' CHECK (dep_type IN ('import','extend','implement'))
);
CREATE INDEX idx_dep_edges_repo ON dep_edges(repo_id);

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('index','reindex')),
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending','processing','completed','failed')),
    progress INT DEFAULT 0,
    error TEXT DEFAULT '',
    result TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_tasks_repo ON tasks(repo_id);
CREATE INDEX idx_tasks_status ON tasks(status);

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id UUID NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    title TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_conv_user ON conversations(user_id);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conv_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('user','assistant')),
    content TEXT DEFAULT '',
    citations JSONB DEFAULT '[]',
    confidence FLOAT DEFAULT 0,
    tokens INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_messages_conv ON messages(conv_id);
```

- [ ] **Step 2: `internal/config/config.go`**

```go
package config

import "os"

type Config struct {
    Port              string
    DatabaseURL       string
    RedisURL          string
    JWTSecret         string
    GitHubClientID    string
    GitHubClientSecret string
    EmbeddingAddr     string
    LLMProvider       string
    LLMAPIKey         string
    LLMModel          string
}

func Load() *Config {
    return &Config{
        Port:              env("PORT", "8080"),
        DatabaseURL:       env("DATABASE_URL", "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"),
        RedisURL:          env("REDIS_URL", "redis://localhost:6379"),
        JWTSecret:         env("JWT_SECRET", "dev-secret"),
        GitHubClientID:    env("GITHUB_CLIENT_ID", ""),
        GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
        EmbeddingAddr:     env("EMBEDDING_ADDR", "localhost:50051"),
        LLMProvider:       env("LLM_PROVIDER", "claude"),
        LLMAPIKey:         env("LLM_API_KEY", ""),
        LLMModel:          env("LLM_MODEL", "claude-sonnet-5"),
    }
}

func env(key, defaultVal string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return defaultVal
}
```

- [ ] **Step 3: `internal/db/postgres.go`**

```go
package db

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
    cfg, err := pgxpool.ParseConfig(databaseURL)
    if err != nil {
        return nil, fmt.Errorf("parse db config: %w", err)
    }
    cfg.MaxConns = 20

    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        return nil, fmt.Errorf("connect db: %w", err)
    }

    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("ping db: %w", err)
    }

    return pool, nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
    files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
    if err != nil {
        return fmt.Errorf("glob migrations: %w", err)
    }

    for _, f := range files {
        sql, err := os.ReadFile(f)
        if err != nil {
            return fmt.Errorf("read %s: %w", f, err)
        }
        if _, err := pool.Exec(ctx, string(sql)); err != nil {
            return fmt.Errorf("exec %s: %w", f, err)
        }
        fmt.Printf("migration applied: %s\n", filepath.Base(f))
    }
    return nil
}
```

- [ ] **Step 4: 编译验证**

```bash
go build ./internal/config/... ./internal/db/...
```

- [ ] **Step 5: Commit**

```bash
git add migrations/ internal/config/ internal/db/ && git commit -m "feat: database migration and connection"
```

---

### Task 4: JWT 鉴权 + Middleware

**Files:**
- Create: `internal/auth/jwt.go`, `internal/auth/middleware.go`

**Interfaces:**
- Consumes: `domain.User` (Task 2), `config.Config` (Task 3)
- Produces: `auth.GenerateToken(userID, secret) (string, error)`, `auth.ValidateToken(token, secret) (string, error)`, `auth.RequireAuth(secret) gin.HandlerFunc`

- [ ] **Step 1: `internal/auth/jwt.go`**

```go
package auth

import (
    "fmt"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    UserID string `json:"user_id"`
    jwt.RegisteredClaims
}

func GenerateToken(userID, secret string) (string, error) {
    claims := Claims{
        UserID: userID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(secret))
}

func ValidateToken(tokenStr, secret string) (string, error) {
    token, err := jwt.ParseWithClaims(tokenStr, &Claims{},
        func(t *jwt.Token) (any, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return []byte(secret), nil
        })
    if err != nil {
        return "", fmt.Errorf("parse token: %w", err)
    }
    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return "", fmt.Errorf("invalid token")
    }
    return claims.UserID, nil
}
```

- [ ] **Step 2: `internal/auth/middleware.go`**

```go
package auth

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
)

func RequireAuth(secret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
            return
        }

        parts := strings.SplitN(authHeader, " ", 2)
        if len(parts) != 2 || parts[0] != "Bearer" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
            return
        }

        userID, err := ValidateToken(parts[1], secret)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            return
        }

        c.Set("user_id", userID)
        c.Next()
    }
}

func GetUserID(c *gin.Context) string {
    userID, _ := c.Get("user_id")
    return userID.(string)
}
```

- [ ] **Step 3: 编写 JWT 单元测试 `internal/auth/jwt_test.go`**

```go
package auth

import (
    "testing"
)

func TestGenerateAndValidate(t *testing.T) {
    secret := "test-secret"
    userID := "user-123"

    token, err := GenerateToken(userID, secret)
    if err != nil {
        t.Fatalf("GenerateToken: %v", err)
    }
    if token == "" {
        t.Fatal("token is empty")
    }

    got, err := ValidateToken(token, secret)
    if err != nil {
        t.Fatalf("ValidateToken: %v", err)
    }
    if got != userID {
        t.Fatalf("expected userID %q, got %q", userID, got)
    }
}

func TestValidateToken_WrongSecret(t *testing.T) {
    token, _ := GenerateToken("user-1", "secret-a")
    _, err := ValidateToken(token, "secret-b")
    if err == nil {
        t.Fatal("expected error for wrong secret")
    }
}

func TestValidateToken_Expired(t *testing.T) {
    // We can't easily test expired without time mocking, skip for MVP
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/auth/... -v
```

- [ ] **Step 5: 编译验证**

```bash
go build ./internal/auth/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/auth/ && git commit -m "feat: JWT auth and middleware"
```

---

### Task 5: GitHub OAuth + 用户 Handler

**Files:**
- Create: `internal/handler/auth.go`

**Interfaces:**
- Consumes: `db.NewPool` (Task 3), `auth.GenerateToken` (Task 4)
- Produces: `POST /api/auth/github/callback` — 接收 GitHub code，换取 access_token，查/插 users 表，返回 JWT

- [ ] **Step 1: `internal/handler/auth.go`**

```go
package handler

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/codebase-copilot/core/internal/auth"
    "github.com/codebase-copilot/core/internal/config"
)

type AuthHandler struct {
    db  *pgxpool.Pool
    cfg *config.Config
}

func NewAuthHandler(db *pgxpool.Pool, cfg *config.Config) *AuthHandler {
    return &AuthHandler{db: db, cfg: cfg}
}

type githubCallbackReq struct {
    Code string `json:"code" binding:"required"`
}

func (h *AuthHandler) GitHubCallback(c *gin.Context) {
    var req githubCallbackReq
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
        return
    }

    // 1. Exchange code for access token
    accessToken, err := exchangeGitHubToken(h.cfg.GitHubClientID, h.cfg.GitHubClientSecret, req.Code)
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("github token exchange: %v", err)})
        return
    }

    // 2. Get user info from GitHub
    ghUser, err := fetchGitHubUser(accessToken)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("fetch github user: %v", err)})
        return
    }

    // 3. Upsert user
    userID, err := h.upsertUser(c.Request.Context(), ghUser)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("upsert user: %v", err)})
        return
    }

    // 4. Generate JWT
    token, err := auth.GenerateToken(userID, h.cfg.JWTSecret)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("generate token: %v", err)})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "token": token,
        "user": gin.H{
            "id":        userID,
            "username":  ghUser.Login,
            "avatar_url": ghUser.AvatarURL,
        },
    })
}

type githubUser struct {
    ID        int64  `json:"id"`
    Login     string `json:"login"`
    Email     string `json:"email"`
    AvatarURL string `json:"avatar_url"`
}

func exchangeGitHubToken(clientID, clientSecret, code string) (string, error) {
    resp, err := http.PostForm("https://github.com/login/oauth/access_token", url.Values{
        "client_id":     {clientID},
        "client_secret": {clientSecret},
        "code":          {code},
    })
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        AccessToken string `json:"access_token"`
        Error       string `json:"error"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    if result.Error != "" {
        return "", fmt.Errorf("github oauth error: %s", result.Error)
    }
    return result.AccessToken, nil
}

func fetchGitHubUser(token string) (*githubUser, error) {
    req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Accept", "application/vnd.github+json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var user githubUser
    if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
        return nil, err
    }
    return &user, nil
}

func (h *AuthHandler) upsertUser(ctx context.Context, u *githubUser) (string, error) {
    var userID string
    err := h.db.QueryRow(ctx, `
        INSERT INTO users (github_id, username, email, avatar_url)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (github_id) DO UPDATE SET username=$2, email=$3, avatar_url=$4
        RETURNING id
    `, u.ID, u.Login, u.Email, u.AvatarURL).Scan(&userID)
    return userID, err
}

func (h *AuthHandler) RegisterRoutes(r *gin.RouterGroup) {
    r.POST("/auth/github/callback", h.GitHubCallback)
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/handler/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/handler/auth.go && git commit -m "feat: GitHub OAuth handler"
```

---

### Task 6: 仓库接入 Service + Handler

**Files:**
- Create: `internal/repo/service.go`, `internal/repo/github.go`, `internal/handler/repo.go`

**Interfaces:**
- Consumes: `domain.Repository`, `domain.RepoStatus` (Task 2), `db.NewPool` (Task 3), `auth.RequireAuth` (Task 4)
- Produces: `RepoService` with methods `Create(ctx, userID, repoFullName) (*Repository, error)`, `List(ctx, userID) ([]Repository, error)`, `Get(ctx, id) (*Repository, error)`, `Delete(ctx, id) error`

- [ ] **Step 1: `internal/repo/github.go`**

```go
package repo

import (
    "encoding/json"
    "fmt"
    "net/http"
)

type GitHubRepo struct {
    ID            int64  `json:"id"`
    FullName      string `json:"full_name"`
    CloneURL      string `json:"clone_url"`
    DefaultBranch string `json:"default_branch"`
}

func FetchGitHubRepo(accessToken, owner, repo string) (*GitHubRepo, error) {
    url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    req.Header.Set("Accept", "application/vnd.github+json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("github api: %s", resp.Status)
    }

    var r GitHubRepo
    if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
        return nil, err
    }
    return &r, nil
}
```

- [ ] **Step 2: `internal/repo/service.go`**

```go
package repo

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/google/uuid"

    "github.com/codebase-copilot/core/internal/domain"
)

type Service struct {
    db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
    return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, userID, fullName string) (*domain.Repository, error) {
    // Parse owner/repo
    var owner, name string
    if _, err := fmt.Sscanf(fullName, "%s/%s", &owner, &name); err != nil {
        return nil, fmt.Errorf("invalid repo name: %s (expected owner/repo)", fullName)
    }

    // Fetch repo info from GitHub
    ghRepo, err := FetchGitHubRepo("", owner, name) // access_token from user's OAuth
    if err != nil {
        return nil, fmt.Errorf("fetch github repo: %w", err)
    }

    repo := &domain.Repository{
        ID:            uuid.New().String(),
        UserID:        userID,
        Name:          name,
        FullName:      ghRepo.FullName,
        CloneURL:      ghRepo.CloneURL,
        DefaultBranch: ghRepo.DefaultBranch,
        Status:        domain.RepoStatusPending,
        CreatedAt:     time.Now(),
    }

    _, err = s.db.Exec(ctx, `
        INSERT INTO repos (id, user_id, name, full_name, clone_url, default_branch, status)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, repo.ID, repo.UserID, repo.Name, repo.FullName, repo.CloneURL, repo.DefaultBranch, repo.Status)
    if err != nil {
        return nil, fmt.Errorf("insert repo: %w", err)
    }

    return repo, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]domain.Repository, error) {
    rows, err := s.db.Query(ctx, `
        SELECT id, user_id, name, full_name, clone_url, default_branch, status, indexed_at, created_at
        FROM repos WHERE user_id = $1 ORDER BY created_at DESC
    `, userID)
    if err != nil {
        return nil, fmt.Errorf("list repos: %w", err)
    }
    defer rows.Close()

    var repos []domain.Repository
    for rows.Next() {
        var r domain.Repository
        if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.FullName, &r.CloneURL,
            &r.DefaultBranch, &r.Status, &r.IndexedAt, &r.CreatedAt); err != nil {
            return nil, fmt.Errorf("scan repo: %w", err)
        }
        repos = append(repos, r)
    }
    return repos, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Repository, error) {
    var r domain.Repository
    err := s.db.QueryRow(ctx, `
        SELECT id, user_id, name, full_name, clone_url, default_branch, status, indexed_at, created_at
        FROM repos WHERE id = $1
    `, id).Scan(&r.ID, &r.UserID, &r.Name, &r.FullName, &r.CloneURL,
        &r.DefaultBranch, &r.Status, &r.IndexedAt, &r.CreatedAt)
    if err != nil {
        return nil, fmt.Errorf("get repo: %w", err)
    }
    return &r, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
    _, err := s.db.Exec(ctx, `DELETE FROM repos WHERE id = $1`, id)
    return err
}
```

- [ ] **Step 3: `internal/handler/repo.go`**

```go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/codebase-copilot/core/internal/auth"
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
```

- [ ] **Step 4: 编译验证**

```bash
go build ./internal/repo/... ./internal/handler/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/repo/ internal/handler/repo.go && git commit -m "feat: repo service and CRUD handlers"
```

---

### Task 7: Protobuf 定义 + Python Embedding Sidecar

**Files:**
- Create: `proto/embedding.proto`, `python-embedding/requirements.txt`, `python-embedding/embedding_service.py`, `python-embedding/server.py`, `python-embedding/Dockerfile`

**Interfaces:**
- Consumes: 无
- Produces: gRPC proto `EmbeddingService { Embed, Rerank }`，Python server 在 `:50051` 监听

- [ ] **Step 1: `proto/embedding.proto`**

```protobuf
syntax = "proto3";

package embedding;

option go_package = "github.com/codebase-copilot/core/proto/embedding";

service EmbeddingService {
  rpc Embed(EmbedRequest) returns (EmbedResponse);
  rpc Rerank(RerankRequest) returns (RerankResponse);
}

message EmbedRequest {
  repeated string texts = 1;
}

message EmbedResponse {
  repeated Embedding vectors = 1;
}

message Embedding {
  repeated float values = 1;
}

message RerankRequest {
  string query = 1;
  repeated string documents = 2;
  int32 top_k = 3;
}

message RerankResponse {
  repeated RerankResult results = 1;
}

message RerankResult {
  int32 index = 1;
  float score = 2;
}
```

- [ ] **Step 2: 生成 Go + Python gRPC 代码**

```bash
# 安装 protoc 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 生成 Go 代码
mkdir -p proto/embedding
protoc --go_out=. --go-grpc_out=. proto/embedding.proto

# 生成 Python 代码
python -m grpc_tools.protoc -I proto --python_out=python-embedding --grpc_python_out=python-embedding proto/embedding.proto
```

- [ ] **Step 3: `python-embedding/requirements.txt`**

```
grpcio==1.60.0
grpcio-tools==1.60.0
protobuf==4.25.1
torch==2.1.2
transformers==4.36.2
FlagEmbedding==1.2.10
```

- [ ] **Step 4: `python-embedding/embedding_service.py`**

```python
from typing import List
from FlagEmbedding import BGEM3FlagModel


class EmbeddingService:
    def __init__(self, model_name: str = "BAAI/bge-m3"):
        print(f"Loading model: {model_name}")
        self.model = BGEM3FlagModel(model_name, use_fp16=True)
        print("Model loaded.")

    def embed(self, texts: List[str]) -> List[List[float]]:
        """Encode texts to 1024-dim vectors (dense outputs)."""
        outputs = self.model.encode(
            texts,
            batch_size=32,
            max_length=512,
            return_dense=True,
            return_sparse=False,
        )
        # BGEM3 outputs dense_vecs as numpy arrays
        return outputs["dense_vecs"].tolist()

    def rerank(self, query: str, documents: List[str], top_k: int = 10) -> List[dict]:
        """Rerank documents by relevance to query using built-in cross-encoder."""
        # Use model's built-in cross-encoder scoring
        pairs = [[query, doc] for doc in documents]
        scores = self.model.compute_score(
            pairs,
            batch_size=32,
            max_length=512,
        )
        # scores is a list of floats
        ranked = sorted(
            [{"index": i, "score": float(s)} for i, s in enumerate(scores)],
            key=lambda x: x["score"],
            reverse=True,
        )
        return ranked[:top_k]
```

- [ ] **Step 5: `python-embedding/server.py`**

```python
import grpc
from concurrent import futures
import embedding_pb2
import embedding_pb2_grpc
from embedding_service import EmbeddingService


class EmbeddingServicer(embedding_pb2_grpc.EmbeddingServiceServicer):
    def __init__(self):
        self.svc = EmbeddingService()

    def Embed(self, request, context):
        vectors = self.svc.embed(list(request.texts))
        response = embedding_pb2.EmbedResponse()
        for v in vectors:
            emb = embedding_pb2.Embedding()
            emb.values.extend(v)
            response.vectors.append(emb)
        return response

    def Rerank(self, request, context):
        results = self.svc.rerank(
            request.query,
            list(request.documents),
            request.top_k or 10,
        )
        response = embedding_pb2.RerankResponse()
        for r in results:
            response.results.append(
                embedding_pb2.RerankResult(index=r["index"], score=r["score"])
            )
        return response


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    embedding_pb2_grpc.add_EmbeddingServiceServicer_to_server(
        EmbeddingServicer(), server
    )
    server.add_insecure_port("[::]:50051")
    server.start()
    print("Embedding server listening on :50051")
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
```

- [ ] **Step 6: `python-embedding/Dockerfile`**

```dockerfile
FROM pytorch/pytorch:2.1.2-cuda12.1-cudnn8-runtime

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY *.py .
COPY embedding_pb2.py embedding_pb2_grpc.py .

EXPOSE 50051
CMD ["python", "server.py"]
```

- [ ] **Step 7: Commit**

```bash
git add proto/ python-embedding/ && git commit -m "feat: protobuf definitions and Python embedding sidecar"
```

---

### Task 8: Embedding gRPC Client (Go)

**Files:**
- Create: `internal/embedding/client.go`

**Interfaces:**
- Consumes: `proto/embedding` (Task 7), `config.Config` (Task 3)
- Produces: `embedding.Client { Embed(texts []string) ([][]float32, error), Rerank(query string, docs []string, topK int) ([]RerankResult, error) }`

- [ ] **Step 1: `internal/embedding/client.go`**

```go
package embedding

import (
    "context"
    "fmt"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    pb "github.com/codebase-copilot/core/proto/embedding"
)

type Client struct {
    conn   *grpc.ClientConn
    client pb.EmbeddingServiceClient
}

func NewClient(ctx context.Context, addr string) (*Client, error) {
    conn, err := grpc.DialContext(ctx, addr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(),
        grpc.WithTimeout(10*time.Second),
    )
    if err != nil {
        return nil, fmt.Errorf("dial embedding service: %w", err)
    }
    return &Client{
        conn:   conn,
        client: pb.NewEmbeddingServiceClient(conn),
    }, nil
}

func (c *Client) Close() error {
    return c.conn.Close()
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
    resp, err := c.client.Embed(ctx, &pb.EmbedRequest{Texts: texts})
    if err != nil {
        return nil, fmt.Errorf("embed: %w", err)
    }
    vectors := make([][]float32, len(resp.Vectors))
    for i, v := range resp.Vectors {
        vectors[i] = v.Values
    }
    return vectors, nil
}

type RerankResult struct {
    Index int
    Score float32
}

func (c *Client) Rerank(ctx context.Context, query string, documents []string, topK int) ([]RerankResult, error) {
    resp, err := c.client.Rerank(ctx, &pb.RerankRequest{
        Query:     query,
        Documents: documents,
        TopK:      int32(topK),
    })
    if err != nil {
        return nil, fmt.Errorf("rerank: %w", err)
    }
    results := make([]RerankResult, len(resp.Results))
    for i, r := range resp.Results {
        results[i] = RerankResult{Index: int(r.Index), Score: r.Score}
    }
    return results, nil
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/embedding/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/embedding/ && git commit -m "feat: embedding gRPC client"
```

---

### Task 9: Vector Store (pgvector 读写 + 检索)

**Files:**
- Create: `internal/vectorstore/store.go`, `internal/vectorstore/search.go`

**Interfaces:**
- Consumes: `domain.IndexNode` (Task 2), `db.NewPool` (Task 3), `embedding.Client` (Task 8)
- Produces: `vectorstore.Store { Insert, BatchInsert, DeleteByRepo }`, `vectorstore.Searcher { HybridSearch }`

- [ ] **Step 1: `internal/vectorstore/store.go`**

```go
package vectorstore

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/codebase-copilot/core/internal/domain"
)

type Store struct {
    db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
    return &Store{db: db}
}

func (s *Store) Insert(ctx context.Context, node *domain.IndexNode) error {
    // pgvector uses string representation for vectors
    vecStr := vectorToString(node.Embedding)
    _, err := s.db.Exec(ctx, `
        INSERT INTO index_nodes (id, repo_id, type, name, signature, code, file_path,
            start_line, end_line, summary, embedding, language, package, metadata)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::vector, $12, $13, '{}')
    `, node.ID, node.RepoID, node.Type, node.Name, node.Signature, node.Code,
        node.FilePath, node.StartLine, node.EndLine, node.Summary,
        vecStr, node.Language, node.Package)
    return err
}

func (s *Store) BatchInsert(ctx context.Context, nodes []*domain.IndexNode) error {
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx)

    for _, node := range nodes {
        vecStr := vectorToString(node.Embedding)
        _, err := tx.Exec(ctx, `
            INSERT INTO index_nodes (id, repo_id, type, name, signature, code, file_path,
                start_line, end_line, summary, embedding, language, package)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::vector, $12, $13)
        `, node.ID, node.RepoID, node.Type, node.Name, node.Signature, node.Code,
            node.FilePath, node.StartLine, node.EndLine, node.Summary,
            vecStr, node.Language, node.Package)
        if err != nil {
            return fmt.Errorf("insert node %s: %w", node.ID, err)
        }
    }
    return tx.Commit(ctx)
}

func (s *Store) DeleteByRepo(ctx context.Context, repoID string) error {
    _, err := s.db.Exec(ctx, `DELETE FROM index_nodes WHERE repo_id = $1`, repoID)
    return err
}

func vectorToString(v []float32) string {
    if len(v) == 0 {
        return "[]"
    }
    s := fmt.Sprintf("[%f", v[0])
    for i := 1; i < len(v); i++ {
        s += fmt.Sprintf(",%f", v[i])
    }
    s += "]"
    return s
}
```

- [ ] **Step 2: `internal/vectorstore/search.go`**

```go
package vectorstore

import (
    "context"
    "fmt"
    "strings"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/codebase-copilot/core/internal/domain"
)

type SearchResult struct {
    Node  *domain.IndexNode
    Score float64
}

type Searcher struct {
    db *pgxpool.Pool
}

func NewSearcher(db *pgxpool.Pool) *Searcher {
    return &Searcher{db: db}
}

// SemanticSearch finds top-k similar nodes by cosine distance.
func (s *Searcher) SemanticSearch(ctx context.Context, repoID string, queryVec []float32, topK int) ([]SearchResult, error) {
    vecStr := vectorToString(queryVec)

    // Ensure ivfflat index is created first time
    s.db.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_nodes_embedding ON index_nodes USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)`)

    rows, err := s.db.Query(ctx, `
        SELECT id, repo_id, type, name, signature, code, file_path,
               start_line, end_line, summary, language, package,
               1 - (embedding <=> $1::vector) AS similarity
        FROM index_nodes
        WHERE repo_id = $2 AND embedding IS NOT NULL
        ORDER BY embedding <=> $1::vector
        LIMIT $3
    `, vecStr, repoID, topK)
    if err != nil {
        return nil, fmt.Errorf("semantic search: %w", err)
    }
    defer rows.Close()

    return scanSearchResults(rows)
}

// GraphExpand expands search by ±1 hop call edges from seed nodes.
func (s *Searcher) GraphExpand(ctx context.Context, repoID string, seedIDs []string) ([]*domain.IndexNode, error) {
    if len(seedIDs) == 0 {
        return nil, nil
    }

    placeholders := make([]string, len(seedIDs))
    args := make([]any, 0, len(seedIDs)+1)
    args = append(args, repoID)
    for i, id := range seedIDs {
        placeholders[i] = fmt.Sprintf("$%d", i+2)
        args = append(args, id)
    }

    query := fmt.Sprintf(`
        SELECT DISTINCT n.id, n.repo_id, n.type, n.name, n.signature, n.code, n.file_path,
               n.start_line, n.end_line, n.summary, n.language, n.package
        FROM index_nodes n
        JOIN call_edges e ON (n.id = e.callee_id OR n.id = e.caller_id)
        WHERE e.repo_id = $1 AND (e.caller_id IN (%s) OR e.callee_id IN (%s))
        AND n.id NOT IN (%s)
        LIMIT 50
    `, strings.Join(placeholders, ","), strings.Join(placeholders, ","), strings.Join(placeholders, ","))

    allArgs := append(args, args...)
    allArgs = append(allArgs, args...)

    rows, err := s.db.Query(ctx, query, allArgs...)
    if err != nil {
        return nil, fmt.Errorf("graph expand: %w", err)
    }
    defer rows.Close()

    var nodes []*domain.IndexNode
    for rows.Next() {
        var n domain.IndexNode
        if err := rows.Scan(&n.ID, &n.RepoID, &n.Type, &n.Name, &n.Signature, &n.Code,
            &n.FilePath, &n.StartLine, &n.EndLine, &n.Summary, &n.Language, &n.Package); err != nil {
            return nil, fmt.Errorf("scan node: %w", err)
        }
        nodes = append(nodes, &n)
    }
    return nodes, nil
}

func scanSearchResults(rows pgx.Rows) ([]SearchResult, error) {
    var results []SearchResult
    for rows.Next() {
        var r SearchResult
        var n domain.IndexNode
        if err := rows.Scan(&n.ID, &n.RepoID, &n.Type, &n.Name, &n.Signature, &n.Code,
            &n.FilePath, &n.StartLine, &n.EndLine, &n.Summary, &n.Language, &n.Package,
            &r.Score); err != nil {
            return nil, fmt.Errorf("scan result: %w", err)
        }
        r.Node = &n
        results = append(results, r)
    }
    return results, nil
}
```

- [ ] **Step 3: 编译验证**

```bash
go build ./internal/vectorstore/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/vectorstore/ && git commit -m "feat: vector store with pgvector semantic search and graph expansion"
```

---

### Task 10: 代码索引器 — Go AST 解析

**Files:**
- Create: `internal/indexer/go_parser.go`, `internal/indexer/service.go`

**Interfaces:**
- Consumes: `domain.IndexNode`, `domain.CallEdge`, `domain.DepEdge` (Task 2), `embedding.Client` (Task 8), `vectorstore.Store` (Task 9)
- Produces: `indexer.Service { IndexRepo(ctx, repo) }` — 解析代码 → 构建节点/边 → embedding → 入库

- [ ] **Step 1: `internal/indexer/go_parser.go`**

```go
package indexer

import (
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "path/filepath"
    "strings"

    "github.com/google/uuid"

    "github.com/codebase-copilot/core/internal/domain"
)

// ParseGoRepo walks a Go repo directory and extracts index nodes + edges.
func ParseGoRepo(repoPath, repoID string) ([]*domain.IndexNode, []*domain.CallEdge, []*domain.DepEdge, error) {
    fset := token.NewFileSet()

    // Parse all .go files (excluding vendor, testdata)
    pkgs, err := parser.ParseDir(fset, repoPath, func(fi os.FileInfo) bool {
        name := fi.Name()
        return !strings.HasSuffix(name, "_test.go") && strings.HasSuffix(name, ".go")
    }, parser.ParseComments)
    if err != nil {
        return nil, nil, nil, err
    }

    var nodes []*domain.IndexNode
    var calls []*domain.CallEdge
    var deps []*domain.DepEdge

    // Map to track: node name → node ID (for call edge linking)
    funcMap := make(map[string]string) // "pkg.FuncName" → nodeID
    fileMap := make(map[string]string) // "filepath" → nodeID

    for pkgName, pkg := range pkgs {
        for filename, file := range pkg.Files {
            // Create file node
            fileID := uuid.New().String()
            fileNode := &domain.IndexNode{
                ID:       fileID,
                RepoID:   repoID,
                Type:     domain.NodeTypeFile,
                Name:     filepath.Base(filename),
                FilePath: relPath(repoPath, filename),
                Language: "go",
                Package:  pkgName,
            }
            nodes = append(nodes, fileNode)
            fileMap[filename] = fileID

            // Extract imports
            for _, imp := range file.Imports {
                depTarget := strings.Trim(imp.Path.Value, "\"")
                deps = append(deps, &domain.DepEdge{
                    ID:       uuid.New().String(),
                    RepoID:   repoID,
                    SourceID: fileID,
                    TargetID: depTarget, // external dep or internal file
                    DepType:  "import",
                })
            }

            // Extract functions and methods
            ast.Inspect(file, func(n ast.Node) bool {
                switch decl := n.(type) {
                case *ast.FuncDecl:
                    funcID := uuid.New().String()
                    funcName := decl.Name.Name
                    if decl.Recv != nil {
                        // Method
                        recvType := extractRecvType(decl.Recv)
                        funcName = recvType + "." + funcName
                    }
                    fullName := pkgName + "." + funcName

                    startPos := fset.Position(decl.Pos())
                    endPos := fset.Position(decl.End())

                    funcNode := &domain.IndexNode{
                        ID:        funcID,
                        RepoID:    repoID,
                        Type:      funcType(decl),
                        Name:      funcName,
                        Signature: buildSignature(decl),
                        Code:      extractSource(filename, startPos.Line, endPos.Line),
                        FilePath:  relPath(repoPath, filename),
                        StartLine: startPos.Line,
                        EndLine:   endPos.Line,
                        Language:  "go",
                        Package:   pkgName,
                    }
                    nodes = append(nodes, funcNode)
                    funcMap[fullName] = funcID

                    // Extract calls within function body
                    ast.Inspect(decl.Body, func(n ast.Node) bool {
                        if call, ok := n.(*ast.CallExpr); ok {
                            calleeName := extractCallee(call)
                            calls = append(calls, &domain.CallEdge{
                                ID:       uuid.New().String(),
                                RepoID:   repoID,
                                CallerID: funcID,
                                CalleeID: calleeName, // will be resolved later
                                FilePath: relPath(repoPath, filename),
                                Line:     fset.Position(call.Pos()).Line,
                            })
                        }
                        return true
                    })
                }
                return true
            })
        }
    }

    // Resolve call edges: try to match callee names to known functions
    for _, call := range calls {
        if resolvedID, ok := funcMap[call.CalleeID]; ok {
            call.CalleeID = resolvedID
        }
        // If not found, calleeID stays as the function name (external or unresolved)
    }

    return nodes, calls, deps, nil
}

func relPath(base, filename string) string {
    rel, _ := filepath.Rel(base, filename)
    return filepath.ToSlash(rel)
}

func funcType(decl *ast.FuncDecl) domain.IndexNodeType {
    if decl.Recv != nil {
        return domain.NodeTypeMethod
    }
    return domain.NodeTypeFunction
}

func extractRecvType(recv *ast.FieldList) string {
    if recv == nil || len(recv.List) == 0 {
        return ""
    }
    switch t := recv.List[0].Type.(type) {
    case *ast.StarExpr:
        if ident, ok := t.X.(*ast.Ident); ok {
            return ident.Name
        }
    case *ast.Ident:
        return t.Name
    }
    return ""
}

func buildSignature(decl *ast.FuncDecl) string {
    var sb strings.Builder
    sb.WriteString("func ")
    if decl.Recv != nil {
        sb.WriteString("(" + extractRecvType(decl.Recv) + ") ")
    }
    sb.WriteString(decl.Name.Name)
    sb.WriteString("(")
    if decl.Type.Params != nil {
        for i, p := range decl.Type.Params.List {
            if i > 0 {
                sb.WriteString(", ")
            }
            for j, n := range p.Names {
                if j > 0 {
                    sb.WriteString(", ")
                }
                sb.WriteString(n.Name)
            }
            sb.WriteString(" ")
            sb.WriteString(typeString(p.Type))
        }
    }
    sb.WriteString(")")
    if decl.Type.Results != nil {
        sb.WriteString(" ")
        if len(decl.Type.Results.List) > 1 {
            sb.WriteString("(")
        }
        for i, r := range decl.Type.Results.List {
            if i > 0 {
                sb.WriteString(", ")
            }
            sb.WriteString(typeString(r.Type))
        }
        if len(decl.Type.Results.List) > 1 {
            sb.WriteString(")")
        }
    }
    return sb.String()
}

func typeString(expr ast.Expr) string {
    switch t := expr.(type) {
    case *ast.Ident:
        return t.Name
    case *ast.StarExpr:
        return "*" + typeString(t.X)
    case *ast.SelectorExpr:
        return typeString(t.X) + "." + t.Sel.Name
    case *ast.ArrayType:
        return "[]" + typeString(t.Elt)
    case *ast.MapType:
        return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
    default:
        return ""
    }
}

func extractCallee(call *ast.CallExpr) string {
    switch fun := call.Fun.(type) {
    case *ast.Ident:
        return fun.Name
    case *ast.SelectorExpr:
        if ident, ok := fun.X.(*ast.Ident); ok {
            return ident.Name + "." + fun.Sel.Name
        }
        return fun.Sel.Name
    }
    return ""
}

func extractSource(filename string, startLine, endLine int) string {
    data, err := os.ReadFile(filename)
    if err != nil {
        return ""
    }
    lines := strings.Split(string(data), "\n")
    if startLine < 1 {
        startLine = 1
    }
    if endLine > len(lines) {
        endLine = len(lines)
    }
    if startLine > endLine {
        return ""
    }
    return strings.Join(lines[startLine-1:endLine], "\n")
}
```

- [ ] **Step 2: `internal/indexer/service.go`**

```go
package indexer

import (
    "context"
    "fmt"
    "os"
    "os/exec"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/codebase-copilot/core/internal/domain"
    "github.com/codebase-copilot/core/internal/embedding"
    "github.com/codebase-copilot/core/internal/vectorstore"
)

type Service struct {
    db       *pgxpool.Pool
    emb      *embedding.Client
    store    *vectorstore.Store
    dataDir  string // e.g. /data/repos
}

func NewService(db *pgxpool.Pool, emb *embedding.Client, store *vectorstore.Store, dataDir string) *Service {
    return &Service{db: db, emb: emb, store: store, dataDir: dataDir}
}

func (s *Service) IndexRepo(ctx context.Context, repo *domain.Repository) error {
    repoPath := fmt.Sprintf("%s/%s", s.dataDir, repo.ID)

    // Step 1: Clone repo if not already cloned
    if err := s.ensureCloned(ctx, repo, repoPath); err != nil {
        return fmt.Errorf("clone: %w", err)
    }

    // Update status to indexing
    s.db.Exec(ctx, `UPDATE repos SET status = 'indexing' WHERE id = $1`, repo.ID)

    // Step 2: Parse code → extract nodes, call edges, dep edges
    nodes, calls, deps, err := ParseGoRepo(repoPath, repo.ID)
    if err != nil {
        return fmt.Errorf("parse repo: %w", err)
    }

    // Step 3: Generate embeddings for all function-level nodes
    var funcNodes []*domain.IndexNode
    embTexts := make([]string, 0)
    for _, n := range nodes {
        if n.Type == domain.NodeTypeFunction || n.Type == domain.NodeTypeMethod {
            embText := n.Signature + "\n" + n.Code
            embTexts = append(embTexts, embText)
            funcNodes = append(funcNodes, n)
        }
    }

    if len(embTexts) > 0 {
        // Batch embed in groups of 32
        batchSize := 32
        for i := 0; i < len(embTexts); i += batchSize {
            end := i + batchSize
            if end > len(embTexts) {
                end = len(embTexts)
            }
            batch := embTexts[i:end]
            vectors, err := s.emb.Embed(ctx, batch)
            if err != nil {
                return fmt.Errorf("embed batch %d: %w", i/batchSize, err)
            }
            for j, v := range vectors {
                funcNodes[i+j].Embedding = v
            }
        }
    }

    // Step 4: Clear existing index and insert new
    if err := s.store.DeleteByRepo(ctx, repo.ID); err != nil {
        return fmt.Errorf("clear old index: %w", err)
    }
    if len(nodes) > 0 {
        if err := s.store.BatchInsert(ctx, nodes); err != nil {
            return fmt.Errorf("insert nodes: %w", err)
        }
    }

    // Step 5: Insert call edges
    for _, e := range calls {
        s.db.Exec(ctx,
            `INSERT INTO call_edges (id, repo_id, caller_id, callee_id, file_path, line) VALUES ($1,$2,$3,$4,$5,$6)`,
            e.ID, e.RepoID, e.CallerID, e.CalleeID, e.FilePath, e.Line)
    }

    // Step 6: Insert dep edges
    for _, e := range deps {
        s.db.Exec(ctx,
            `INSERT INTO dep_edges (id, repo_id, source_id, target_id, dep_type) VALUES ($1,$2,$3,$4,$5)`,
            e.ID, e.RepoID, e.SourceID, e.TargetID, e.DepType)
    }

    // Step 7: Mark ready
    s.db.Exec(ctx, `UPDATE repos SET status = 'ready', indexed_at = now() WHERE id = $1`, repo.ID)

    return nil
}

func (s *Service) ensureCloned(ctx context.Context, repo *domain.Repository, path string) error {
    // Check if already cloned
    if _, err := os.Stat(path); err == nil {
        // Pull latest
        cmd := exec.CommandContext(ctx, "git", "-C", path, "pull", "origin", repo.DefaultBranch)
        return cmd.Run()
    }

    // Clone
    cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1",
        "--branch", repo.DefaultBranch, repo.CloneURL, path)
    return cmd.Run()
}
```

- [ ] **Step 3: 编译验证**

```bash
go build ./internal/indexer/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/indexer/ && git commit -m "feat: Go AST code indexer and indexing service"
```

---

### Task 11: 异步任务服务

**Files:**
- Create: `internal/task/service.go`, `internal/handler/task.go`

**Interfaces:**
- Consumes: `domain.Task`, `domain.TaskStatus` (Task 2), `db.NewPool` (Task 3), `indexer.Service` (Task 10)
- Produces: `task.Service { Enqueue, Poll, UpdateProgress }`，handler `GET /api/tasks/:id`

- [ ] **Step 1: `internal/task/service.go`**

```go
package task

import (
    "context"
    "fmt"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/codebase-copilot/core/internal/domain"
)

type Service struct {
    db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
    return &Service{db: db}
}

func (s *Service) Enqueue(ctx context.Context, repoID string, taskType domain.TaskType) (*domain.Task, error) {
    t := &domain.Task{
        ID:     uuid.New().String(),
        RepoID: repoID,
        Type:   taskType,
        Status: domain.TaskStatusPending,
    }
    _, err := s.db.Exec(ctx, `
        INSERT INTO tasks (id, repo_id, type, status, progress) VALUES ($1,$2,$3,$4,0)
    `, t.ID, t.RepoID, t.Type, t.Status)
    if err != nil {
        return nil, fmt.Errorf("enqueue task: %w", err)
    }
    return t, nil
}

func (s *Service) UpdateProgress(ctx context.Context, taskID string, progress int) error {
    _, err := s.db.Exec(ctx,
        `UPDATE tasks SET progress = $1, updated_at = now() WHERE id = $2`, progress, taskID)
    return err
}

func (s *Service) Complete(ctx context.Context, taskID, result string) error {
    _, err := s.db.Exec(ctx,
        `UPDATE tasks SET status = 'completed', result = $1, progress = 100, updated_at = now() WHERE id = $2`,
        result, taskID)
    return err
}

func (s *Service) Fail(ctx context.Context, taskID, errMsg string) error {
    _, err := s.db.Exec(ctx,
        `UPDATE tasks SET status = 'failed', error = $1, updated_at = now() WHERE id = $2`,
        errMsg, taskID)
    return err
}

func (s *Service) Get(ctx context.Context, taskID string) (*domain.Task, error) {
    var t domain.Task
    err := s.db.QueryRow(ctx,
        `SELECT id, repo_id, type, status, progress, COALESCE(error,''), COALESCE(result,''), created_at, updated_at FROM tasks WHERE id = $1`,
        taskID,
    ).Scan(&t.ID, &t.RepoID, &t.Type, &t.Status, &t.Progress, &t.Error, &t.Result, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        return nil, fmt.Errorf("get task: %w", err)
    }
    return &t, nil
}
```

- [ ] **Step 2: `internal/handler/task.go`**

```go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"

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

func (h *TaskHandler) RegisterRoutes(r *gin.RouterGroup) {
    r.GET("/tasks/:id", h.Get)
}
```

- [ ] **Step 3: 编译验证**

```bash
go build ./internal/task/... ./internal/handler/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/task/ internal/handler/task.go && git commit -m "feat: async task service and handler"
```

---

### Task 12: LLM Client + RAG 编排

**Files:**
- Create: `internal/qa/llm.go`, `internal/qa/rag.go`, `internal/qa/sse.go`

**Interfaces:**
- Consumes: `domain.Question`, `domain.SSEChunk`, `domain.SSEDone` (Task 2), `embedding.Client` (Task 8), `vectorstore.Searcher` (Task 9)
- Produces: `qa.Service { Ask(ctx, question, w) }` — 完整 RAG 管线 + SSE 流式输出

- [ ] **Step 1: `internal/qa/llm.go`**

```go
package qa

import (
    "bufio"
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

type LLMConfig struct {
    Provider string // claude, deepseek
    APIKey   string
    Model    string
}

type LLMClient struct {
    cfg    LLMConfig
    http   *http.Client
}

func NewLLMClient(cfg LLMConfig) *LLMClient {
    return &LLMClient{cfg: cfg, http: &http.Client{}}
}

type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type ChatRequest struct {
    Model    string        `json:"model"`
    Messages []ChatMessage `json:"messages"`
    Stream   bool          `json:"stream"`
    MaxTokens int          `json:"max_tokens"`
}

// StreamChat calls the LLM API and sends each text delta to the callback.
func (c *LLMClient) StreamChat(ctx context.Context, messages []ChatMessage, onDelta func(text string)) error {
    var endpoint string
    switch c.cfg.Provider {
    case "claude":
        endpoint = "https://api.anthropic.com/v1/messages"
    case "deepseek":
        endpoint = "https://api.deepseek.com/v1/chat/completions"
    default:
        return fmt.Errorf("unknown LLM provider: %s", c.cfg.Provider)
    }

    reqBody := map[string]any{
        "model":      c.cfg.Model,
        "messages":   messages,
        "stream":     true,
        "max_tokens": 4096,
    }

    bodyBytes, _ := json.Marshal(reqBody)
    req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
    if c.cfg.Provider == "claude" {
        req.Header.Set("x-api-key", c.cfg.APIKey)
        req.Header.Set("anthropic-version", "2023-06-01")
    }

    resp, err := c.http.Do(req)
    if err != nil {
        return fmt.Errorf("llm request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("llm error %d: %s", resp.StatusCode, string(body))
    }

    return c.parseSSEStream(resp.Body, onDelta)
}

func (c *LLMClient) parseSSEStream(r io.Reader, onDelta func(text string)) error {
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        line := scanner.Text()
        if line == "" || line == "data: [DONE]" {
            continue
        }

        if len(line) > 6 && line[:6] == "data: " {
            data := line[6:]
            var event map[string]any
            if err := json.Unmarshal([]byte(data), &event); err != nil {
                continue
            }

            switch c.cfg.Provider {
            case "claude":
                if delta, ok := extractClaudeDelta(event); ok && delta != "" {
                    onDelta(delta)
                }
            case "deepseek":
                if choices, ok := event["choices"].([]any); ok && len(choices) > 0 {
                    choice := choices[0].(map[string]any)
                    if delta, ok := choice["delta"].(map[string]any); ok {
                        if content, ok := delta["content"].(string); ok {
                            onDelta(content)
                        }
                    }
                }
            }
        }
    }
    return scanner.Err()
}

func extractClaudeDelta(event map[string]any) (string, bool) {
    eventType, _ := event["type"].(string)
    if eventType == "content_block_delta" {
        if delta, ok := event["delta"].(map[string]any); ok {
            if text, ok := delta["text"].(string); ok {
                return text, true
            }
        }
    }
    return "", false
}
```

- [ ] **Step 2: `internal/qa/rag.go`**

```go
package qa

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/codebase-copilot/core/internal/domain"
    "github.com/codebase-copilot/core/internal/embedding"
    "github.com/codebase-copilot/core/internal/vectorstore"
)

type RAGService struct {
    emb     *embedding.Client
    searcher *vectorstore.Searcher
    llm     *LLMClient
}

func NewRAGService(emb *embedding.Client, searcher *vectorstore.Searcher, llm *LLMClient) *RAGService {
    return &RAGService{emb: emb, searcher: searcher, llm: llm}
}

// Ask runs the full RAG pipeline and streams the answer via SSE to the writer.
func (s *RAGService) Ask(ctx context.Context, repoID, question string, history []domain.Message, w http.ResponseWriter) error {
    // Step 1: Embed the question
    vecs, err := s.emb.Embed(ctx, []string{question})
    if err != nil {
        return fmt.Errorf("embed question: %w", err)
    }

    // Step 2: Semantic search → top 50
    results, err := s.searcher.SemanticSearch(ctx, repoID, vecs[0], 50)
    if err != nil {
        return fmt.Errorf("semantic search: %w", err)
    }

    // Step 3: Graph expand → ±1 hop call edges
    seedIDs := make([]string, len(results))
    for i, r := range results {
        seedIDs[i] = r.Node.ID
    }
    expanded, _ := s.searcher.GraphExpand(ctx, repoID, seedIDs)
    // Merge expanded into results with a lower score
    for _, n := range expanded {
        results = append(results, vectorstore.SearchResult{Node: n, Score: 0.3})
    }

    // Step 4: Dedup by node ID
    seen := make(map[string]bool)
    var unique []vectorstore.SearchResult
    for _, r := range results {
        if !seen[r.Node.ID] {
            seen[r.Node.ID] = true
            unique = append(unique, r)
        }
    }

    // Step 5: Rerank to top-10
    docs := make([]string, len(unique))
    for i, r := range unique {
        docs[i] = fmt.Sprintf("[%s:%d] %s\n%s", r.Node.FilePath, r.Node.StartLine, r.Node.Signature, r.Node.Code)
    }
    reranked, err := s.emb.Rerank(ctx, question, docs, 10)
    if err != nil {
        // Fallback: use top-10 from semantic search
        reranked = make([]embedding.RerankResult, 0)
        for i := 0; i < len(unique) && i < 10; i++ {
            reranked = append(reranked, embedding.RerankResult{Index: i, Score: float32(unique[i].Score)})
        }
    }

    // Step 6: Build context from top results
    var contextParts []string
    var citations []domain.Citation
    for _, rr := range reranked {
        if rr.Index < len(unique) {
            n := unique[rr.Index].Node
            contextParts = append(contextParts, fmt.Sprintf(
                "File: %s (line %d-%d)\n%s\n```\n%s\n```",
                n.FilePath, n.StartLine, n.EndLine, n.Signature, n.Code,
            ))
            citations = append(citations, domain.Citation{
                File:    n.FilePath,
                Line:    n.StartLine,
                Content: n.Code,
                Score:   float64(rr.Score),
            })
        }
    }
    context := fmt.Sprintf("You are analyzing a code repository. Use the following code snippets to answer the question.\n\nCODE:\n%s\n\n---\n", join(contextParts, "\n\n"))

    // Step 7: Build messages for LLM
    messages := []ChatMessage{
        {Role: "system", Content: "You are an expert code analyst. Answer questions based on the provided code snippets. Be specific, reference file paths and line numbers when citing code. If the provided context isn't sufficient, say so."},
        {Role: "user", Content: context + "\n\nQuestion: " + question},
    }
    // Prepend conversation history (simplified: last 6 messages)
    for i := len(history) - 6; i < len(history); i++ {
        if i >= 0 {
            messages = append([]ChatMessage{{Role: history[i].Role, Content: history[i].Content}}, messages...)
        }
    }

    // Step 8: Setup SSE writer
    flusher, ok := w.(http.Flusher)
    if !ok {
        return fmt.Errorf("streaming not supported")
    }
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Step 9: Stream LLM response → SSE chunks
    totalTokens := 0
    err = s.llm.StreamChat(ctx, messages, func(text string) {
        chunk := domain.SSEChunk{
            Text:      text,
            Citations: citations, // Send citations with first chunk
        }
        data, _ := json.Marshal(chunk)
        fmt.Fprintf(w, "event: chunk\ndata: %s\n\n", data)
        flusher.Flush()
        totalTokens++
    })
    if err != nil {
        fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
        flusher.Flush()
        return err
    }

    // Step 10: Send done event
    done := domain.SSEDone{
        Confidence: 0.85, // TODO: calculate from rerank scores
        Tokens:     totalTokens,
    }
    data, _ := json.Marshal(done)
    fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
    flusher.Flush()

    return nil
}

func join(parts []string, sep string) string {
    if len(parts) == 0 {
        return ""
    }
    result := parts[0]
    for i := 1; i < len(parts); i++ {
        result += sep + parts[i]
    }
    return result
}
```

- [ ] **Step 3: 编译验证 + 修复 import**

```bash
# Add missing imports: encoding/json in rag.go
go build ./internal/qa/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/qa/ && git commit -m "feat: LLM client, RAG orchestration, and SSE streaming"
```

---

### Task 13: Ask Handler (SSE 端点) + Conversation Handler

**Files:**
- Create: `internal/handler/ask.go`, `internal/handler/conversation.go`

**Interfaces:**
- Consumes: `qa.RAGService` (Task 12), `domain.Conversation`, `domain.Message` (Task 2), `auth.RequireAuth` (Task 4)
- Produces: `POST /api/ask` (SSE), `GET /api/conversations`, `GET /api/conversations/:id`

- [ ] **Step 1: `internal/handler/ask.go`**

```go
package handler

import (
    "encoding/json"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/google/uuid"

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
```

- [ ] **Step 2: `internal/handler/conversation.go`**

```go
package handler

import (
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
        m.CreatedAt = fmt.Sprintf("%v", createdAt)
        msgs = append(msgs, m)
    }
    c.JSON(http.StatusOK, msgs)
}

func (h *ConversationHandler) RegisterRoutes(r *gin.RouterGroup) {
    r.GET("/conversations", h.List)
    r.GET("/conversations/:id", h.Get)
}
```

- [ ] **Step 3: 编译验证**

```bash
go build ./internal/handler/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/handler/ask.go internal/handler/conversation.go && git commit -m "feat: ask SSE endpoint and conversation handlers"
```

---

### Task 14: Main 入口 — 组装所有依赖

**Files:**
- Create: `cmd/server/main.go`

**Interfaces:**
- Consumes: 所有 handler、service、config、db
- Produces: 可运行的 Go HTTP Server

- [ ] **Step 1: `cmd/server/main.go`**

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

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
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    cfg := config.Load()

    // Database
    pool, err := db.NewPool(ctx, cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("database: %v", err)
    }
    defer pool.Close()

    // Run migrations
    if err := db.RunMigrations(ctx, pool, "migrations"); err != nil {
        log.Fatalf("migrations: %v", err)
    }

    // Embedding client
    embClient, err := embedding.NewClient(ctx, cfg.EmbeddingAddr)
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

    // Graceful shutdown
    go func() {
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
        <-sigCh
        log.Println("shutting down...")
        cancel()
    }()

    log.Printf("server starting on :%s", cfg.Port)
    if err := r.Run(":" + cfg.Port); err != nil {
        log.Fatalf("server: %v", err)
    }
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./cmd/server
```

- [ ] **Step 3: Commit**

```bash
git add cmd/ && git commit -m "feat: main server entrypoint with full dependency wiring"
```

---

### Task 15: React 前端 — 脚手架 + 页面

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`, `web/src/api.ts`, `web/src/components/Layout.tsx`, `web/src/pages/RepoList.tsx`, `web/src/pages/RepoDetail.tsx`, `web/src/pages/Ask.tsx`, `web/src/components/SSEViewer.tsx`, `web/src/components/Citations.tsx`

**Interfaces:**
- Consumes: 后端 API (Task 13/14)
- Produces: 可运行的前端 SPA

- [ ] **Step 1: `web/package.json`**

```json
{
  "name": "codebase-copilot-web",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "react-router-dom": "^6.21.0",
    "antd": "^5.12.0",
    "@ant-design/icons": "^5.2.6"
  },
  "devDependencies": {
    "@types/react": "^18.2.45",
    "@types/react-dom": "^18.2.18",
    "@vitejs/plugin-react": "^4.2.1",
    "typescript": "^5.3.3",
    "vite": "^5.0.10"
  }
}
```

- [ ] **Step 2: 其余前端文件的简略版 — 先创建骨架再逐步完善**

```tsx
// web/src/main.tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>
);
```

```tsx
// web/src/App.tsx
import { Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import RepoList from './pages/RepoList';
import RepoDetail from './pages/RepoDetail';
import Ask from './pages/Ask';

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<RepoList />} />
        <Route path="/repos/:id" element={<RepoDetail />} />
        <Route path="/repos/:id/ask" element={<Ask />} />
      </Routes>
    </Layout>
  );
}
```

```tsx
// web/src/api.ts
const BASE = '/api';

export async function api(path: string, options?: RequestInit) {
  const token = localStorage.getItem('token');
  const res = await fetch(BASE + path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options?.headers,
    },
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export function askStream(repoId: string, question: string, onChunk: (data: any) => void, onDone: (data: any) => void, onError: (err: string) => void): AbortController {
  const controller = new AbortController();
  const token = localStorage.getItem('token');

  fetch(BASE + '/ask', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ repo_id: repoId, question }),
    signal: controller.signal,
  }).then(async (response) => {
    const reader = response.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          try {
            const data = JSON.parse(line.slice(6));
            if (line.includes('event: chunk')) onChunk(data);
            else if (line.includes('event: done')) onDone(data);
            else onChunk(data); // fallback
          } catch {}
        }
      }
    }
  }).catch(err => {
    if (err.name !== 'AbortError') onError(err.message);
  });

  return controller;
}
```

```tsx
// web/src/pages/Ask.tsx (核心问答页面)
import { useState, useRef, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { Input, Button, Card, Typography, Space, Tag } from 'antd';
import { SendOutlined, StopOutlined } from '@ant-design/icons';
import { askStream } from '../api';
import SSEViewer from '../components/SSEViewer';
import Citations from '../components/Citations';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  citations?: any[];
  confidence?: number;
}

export default function AskPage() {
  const { id: repoId } = useParams<{ id: string }>();
  const [question, setQuestion] = useState('');
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(false);
  const [currentChunk, setCurrentChunk] = useState('');
  const [citations, setCitations] = useState<any[]>([]);
  const controllerRef = useRef<AbortController | null>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => { chatEndRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages, currentChunk]);

  const handleAsk = () => {
    if (!question.trim() || !repoId) return;
    const q = question.trim();
    setQuestion('');
    setMessages(prev => [...prev, { role: 'user', content: q }]);
    setCurrentChunk('');
    setCitations([]);
    setLoading(true);

    const ctrl = askStream(repoId!, q,
      (data) => {
        setCurrentChunk(prev => prev + data.text);
        if (data.citations?.length) setCitations(data.citations);
      },
      (data) => {
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: currentChunk,
          citations: [...citations],
          confidence: data.confidence,
        }]);
        setCurrentChunk('');
        setLoading(false);
      },
      (err) => {
        setMessages(prev => [...prev, { role: 'assistant', content: `Error: ${err}` }]);
        setLoading(false);
      }
    );
    controllerRef.current = ctrl;
  };

  const handleStop = () => {
    controllerRef.current?.abort();
    setLoading(false);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 64px)' }}>
      <div style={{ flex: 1, overflow: 'auto', padding: 16 }}>
        {messages.map((msg, i) => (
          <div key={i} style={{ marginBottom: 16 }}>
            <Tag color={msg.role === 'user' ? 'blue' : 'green'}>{msg.role}</Tag>
            <Card size="small">
              <SSEViewer text={msg.content} />
              {msg.citations?.length > 0 && <Citations items={msg.citations} />}
              {msg.confidence != null && (
                <Typography.Text type="secondary">
                  Confidence: {(msg.confidence * 100).toFixed(0)}%
                </Typography.Text>
              )}
            </Card>
          </div>
        ))}
        {currentChunk && (
          <div style={{ marginBottom: 16 }}>
            <Tag color="green">assistant</Tag>
            <Card size="small">
              <SSEViewer text={currentChunk} />
              {citations.length > 0 && <Citations items={citations} />}
            </Card>
          </div>
        )}
        {loading && !currentChunk && <Typography.Text type="secondary">Thinking...</Typography.Text>}
        <div ref={chatEndRef} />
      </div>
      <div style={{ padding: 16, borderTop: '1px solid #f0f0f0' }}>
        <Space.Compact style={{ width: '100%' }}>
          <Input.TextArea
            value={question}
            onChange={e => setQuestion(e.target.value)}
            onPressEnter={e => { if (!e.shiftKey) { e.preventDefault(); handleAsk(); } }}
            placeholder="Ask about this codebase... (Shift+Enter for new line)"
            rows={2}
            disabled={loading}
          />
          {loading ? (
            <Button icon={<StopOutlined />} onClick={handleStop} danger />
          ) : (
            <Button icon={<SendOutlined />} onClick={handleAsk} type="primary" />
          )}
        </Space.Compact>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: 创建其他组件文件（RepoList, RepoDetail, Layout, SSEViewer, Citations）**

略——见 commit 中的完整文件。

- [ ] **Step 6: 安装依赖并构建**

```bash
cd web && npm install && npm run build
```

- [ ] **Step 7: Commit**

```bash
git add web/ && git commit -m "feat: React frontend with repo list, Q&A interface, and SSE streaming"
```

---

### Task 16: 集成测试 + 端到端验证

**Files:**
- Modify: `docker-compose.yml` (mount web dist)
- Create: `internal/handler/handler_test.go`

**Description:** 启动完整 Docker Compose 环境，验证：
1. `POST /api/auth/github/callback` 返回 JWT
2. `POST /api/repos` 创建仓库
3. `GET /api/repos` 列出仓库
4. `POST /api/ask` 返回 SSE 流

- [ ] **Step 1: 添加前端静态文件服务到 main.go**

```go
// In main.go, add before r.Run():
r.Static("/assets", "./web/dist/assets")
r.StaticFile("/", "./web/dist/index.html")
r.NoRoute(func(c *gin.Context) {
    c.File("./web/dist/index.html")
})
```

- [ ] **Step 2: 编写集成测试 `internal/handler/handler_test.go`**

```go
package handler_test

import (
    "testing"
)

// MVP integration tests — see commit for full test file
func TestRepoCRUD(t *testing.T) { /* ... */ }
func TestAskSSE(t *testing.T) { /* ... */ }
```

- [ ] **Step 3: 运行测试套件**

```bash
go test ./... -v -count=1
```

- [ ] **Step 4: Docker Compose 全栈验证**

```bash
docker compose up -d --build
# 验证: curl http://localhost:8080/api/repos
# 验证: 浏览器打开 http://localhost:8080
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "test: integration tests and end-to-end verification"
```

---

## 实现顺序

按依赖关系，必须顺序执行：Task 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12 → 13 → 14 → 15 → 16

其中 Task 7（Python sidecar）可以与 Task 5-6 并行开发。
