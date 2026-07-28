# Codebase Copilot Platform — MVP 设计文档

日期: 2026-07-28
目标: 2-3 个月交付核心链路，后端 + AI 工程化深度优先

---

## 1. 目标与范围

面向 Git 仓库的智能研发平台 MVP。核心链路：

```
GitHub OAuth → Clone 仓库 → 代码分层索引 → pgvector 存储
    → RAG 检索 + Rerank → LLM 流式问答 → Web 控制台展示
```

**MVP 必做：**
- GitHub OAuth 接入 + 仓库克隆
- 代码分层索引（文件 → 函数 → 调用关系图）
- pgvector 向量存储 + 混合检索（语义 + 调用图扩展）
- 流式代码问答（SSE，带引用文件 + 置信度）
- React 前端控制台（简洁问答界面）
- 异步任务队列（索引进度可见）
- 基础多租户隔离（JWT + 用户级）

**MVP 不做/裁剪：**
- Gitee 接入（只做 GitHub）
- 架构图自动生成（后期）
- Docker 沙箱测试执行（后期）
- 完整 RBAC（先用户级隔离）
- Kafka/NATS（用 Redis）
- gRPC 内部通信（先 HTTP）
- Prometheus/Grafana（后期）
- 评测模块（后期）

---

## 2. 技术栈

| 层 | 选型 | 说明 |
|------|------|------|
| 后端框架 | Go 1.22+ + Gin | 高性能 HTTP，生态成熟 |
| 数据库 | PostgreSQL 15 + pgvector | 关系 + 向量统一存储 |
| 缓存/队列 | Redis | 会话缓存 + 异步任务队列 |
| 代码解析 | Go: go/parser + go/ast; JS/TS: tree-sitter | 标准库优先 |
| Embedding | Python sidecar + BGE-M3 (1024维) | 本地部署，gRPC 调用 |
| Rerank | BGE-Reranker (本地 Python sidecar) | 同上 |
| LLM | Claude API / DeepSeek API | 云端推理，Function Calling |
| 前端 | React + Ant Design | 简洁可用 |
| 部署 | Docker Compose | 四容器: Go + PG + Redis + Python |

---

## 3. 架构：模块化单块

```
┌────────────── Go Binary ──────────────┐
│  Gin HTTP Server ─ JWT Middleware      │
│                                         │
│  ┌──────────┐ ┌──────────┐ ┌─────────┐  │
│  │ Repo     │ │ Indexer  │ │ QA      │  │
│  │ Service  │ │ Service  │ │ Service │  │
│  └──────────┘ └──────────┘ └─────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌─────────┐  │
│  │ Auth     │ │ Task     │ │ Vector  │  │
│  │ Service  │ │ Service  │ │ Store   │  │
│  └──────────┘ └──────────┘ └─────────┘  │
│                                         │
│  内部: domain package 通过 interface 解耦  │
├─────────────────────────────────────────┤
│  PostgreSQL + pgvector  │  Redis        │
└─────────────────────────────────────────┘
         ↕ gRPC
┌──────────────────┐
│ Python Embedding │  ← 独立 sidecar (BGE-M3 + Reranker)
│    Service       │
└──────────────────┘
```

- 单 Go 进程，内部按 domain 分包
- 异步任务（索引）用 goroutine 内嵌；后续可抽出独立 Worker
- Python sidecar 专门跑 embedding/rerank 模型

---

## 4. 项目结构

```
codebase-copilot/
├── cmd/server/main.go
├── internal/
│   ├── domain/          # 纯 struct: Repo, IndexNode, CallEdge, Question, User
│   ├── repo/            # GitHub clone + webhook
│   ├── indexer/         # 代码索引管线 (AST + 分层索引)
│   ├── vectorstore/     # pgvector CRUD + 检索
│   ├── embedding/       # gRPC client → Python sidecar
│   ├── qa/              # 问答编排 (RAG + LLM + SSE)
│   ├── task/            # 异步任务管理
│   ├── auth/            # JWT
│   └── handler/         # Gin HTTP handlers (薄层)
├── migrations/          # DDL
├── python-embedding/    # Python sidecar
├── web/                 # React 前端
├── docker-compose.yml
└── Makefile
```

分层规则：
- `domain/` 零外部依赖，纯数据结构
- `*Service/` 业务逻辑层，只依赖 domain 和外部接口
- `handler/` 只做参数校验和响应序列化

---

## 5. 核心数据流

### 5.1 仓库索引（异步）

```
GitHub OAuth → Clone 仓库
  → Go AST / tree-sitter 解析
  → 抽取: FileNode / FuncNode / CallEdge / DepEdge
  → Python Sidecar: BGE-M3 Embedding(函数签名+代码)
  → pgvector 写入 (向量 + 元数据)
  → 更新 repos.status = 'ready'
```

索引粒度：**函数级**（函数签名 + 代码片段 + 所在文件路径）
- FileNode: 文件摘要 + 路径
- FuncNode: 函数/方法签名 + 代码 + 所在文件行号
- CallEdge: caller → callee（函数调用）
- DepEdge: import/依赖关系（文件级别）

### 5.2 问答（实时 Stream）

```
用户提问 "鉴权逻辑在哪？"
  → Embedding(query) via Python sidecar
  → pgvector 混合检索:
    ├── 语义相似度 (cosine_distance + ivfflat 索引)
    ├── 调用图扩展: 命中函数 ±1 跳的 caller/callee
    └── 粗排 Top-50
  → Rerank (BGE-Reranker) 精排 Top-10
  → Context Assembly: 拼装代码片段 + 文件路径 + 元数据
  → LLM (Claude/DeepSeek) 流式生成 + Function Calling
  → SSE 推送: text chunk + citations + confidence
```

### 5.3 SSE 事件格式

```
event: chunk
data: {"text":"鉴权逻辑在","citations":[{"file":"auth/jwt.go","line":42,"score":0.89}]}

event: chunk
data: {"text":" middleware/auth.go 中..."}

event: done
data: {"confidence":0.87,"tokens":1247,"conv_id":"xxx"}
```

---

## 6. 数据库 Schema

```sql
users (id UUID PK, github_id, username, email, avatar_url, created_at)

repos (id UUID PK, user_id FK, name, url, default_branch,
       status TEXT,    -- pending|cloning|indexing|ready|error
       indexed_at, created_at)

index_nodes (id UUID PK, repo_id FK, type TEXT,   -- file|function|class|method
             name TEXT, signature TEXT, code TEXT,
             file_path TEXT, start_line INT, end_line INT,
             summary TEXT,
             embedding vector(1024),    -- pgvector, BGE-M3 = 1024 dims
             metadata jsonb)

call_edges (id UUID PK, repo_id FK, caller_id FK, callee_id FK,
            file_path TEXT, line INT)

dep_edges (id UUID PK, repo_id FK, source_id FK, target_id FK,
           dep_type TEXT)   -- import|extend|implement

tasks (id UUID PK, repo_id FK, type TEXT, status TEXT,
       progress INT, result jsonb, error TEXT, created_at)

conversations (id UUID PK, user_id FK, repo_id FK, title TEXT, created_at)

messages (id UUID PK, conv_id FK, role TEXT, content TEXT,
          citations jsonb, confidence FLOAT, tokens INT, created_at)
```

索引策略：
- `index_nodes(repo_id, type)` 复合索引
- `index_nodes(embedding)` ivfflat 向量索引
- `call_edges(repo_id)`, `dep_edges(repo_id)`

---

## 7. API

```
POST   /api/auth/github/callback      → JWT

GET    /api/repos                     → 用户仓库列表
POST   /api/repos                     → 接入新仓库（启动后台索引）
GET    /api/repos/:id                 → 仓库详情 + 索引进度
DELETE /api/repos/:id

GET    /api/repos/:id/files           → 文件树
GET    /api/repos/:id/graph           → 依赖图数据

POST   /api/ask                       → SSE Stream (Content-Type: text/event-stream)
       body: { repo_id, question, conversation_id? }

GET    /api/conversations             → 对话历史
GET    /api/conversations/:id         → 对话详情

GET    /api/tasks/:id                 → 任务进度
```

---

## 8. 前端页面（简洁版）

- `/` — 仓库列表 + 接入 GitHub 按钮
- `/repos/:id` — 文件树 + 依赖图（轻量可视化）
- `/repos/:id/ask` — 问答界面（主交互页）：左侧代码浏览 / 右侧对话（SSE 流式展示 + 引用高亮）

技术：React + Ant Design，SSE 用 `EventSource` API 接收流式回答

---

## 9. 多租户隔离

- JWT 内带 `user_id`
- Gin middleware 解析 JWT → 注入 context
- 所有查询带上 `WHERE user_id = ?` 或 `WHERE repo_id IN (user repos)`
- 索引数据通过 `repo_id` 隔离

---

## 10. 混合 AI 架构

```
┌──────────────────────────────────────────┐
│                 Go Backend                │
│                                           │
│   ┌─ Embedding Client (gRPC) ──────────┐  │
│   │  Embed(text) → []float32(1024)     │  │
│   │  Rerank(query, docs) → scores[]    │  │
│   └────────────────────────────────────┘  │
│         │                                  │
│   ┌─ LLM Client (HTTP) ─────────────────┐  │
│   │  Chat(messages, tools) → SSE stream │  │
│   │  - Claude API or DeepSeek API       │  │
│   │  - Function Calling: search_code,    │  │
│   │    get_file, list_directory          │  │
│   └──────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
        │ gRPC                    │ HTTP
        ▼                         ▼
┌───────────────┐     ┌──────────────────┐
│ Python Sidecar│     │  Cloud LLM API   │
│ - BGE-M3      │     │  Claude/DeepSeek │
│ - BGE-Reranker│     └──────────────────┘
└───────────────┘
```

---

## 11. 部署 (Docker Compose)

```yaml
services:
  app:       # Go binary, port 8080
  db:        # PostgreSQL 15 + pgvector, port 5432
  redis:     # Redis 7, port 6379
  embedding: # Python gRPC sidecar, port 50051
```

---

## 12. 第二阶段（MVP 之后）

- 自动架构图生成（依赖图可交互可视化）
- Docker 沙箱测试执行
- 评测模块（30-50 样本 + 命中率/引用准确率/耗时/Token 成本统计）
- Prometheus + Grafana + OpenTelemetry 可观测
- 完整 RBAC（团队空间）
- Gitee 支持
- K8s Job 执行
- gRPC 内部通信迁移
