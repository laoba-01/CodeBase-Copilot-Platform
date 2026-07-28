# CodeBase Copilot Platform

面向 Git 仓库的 AI 智能研发平台。接入 GitHub 仓库后，自动分析代码结构、生成索引，支持自然语言流式代码问答。

## 核心功能

- **GitHub 仓库接入** — OAuth 授权，一键 Clone 并启动后台索引
- **代码分层索引** — 按 文件 → 函数 → 调用关系 → 依赖图 四层结构化索引
- **流式代码问答** — RAG 检索 + LLM 推理，前端实时展示引用文件、置信度
- **异步任务队列** — 索引/重索引等耗时任务后台执行，进度轮询可见
- **多租户隔离** — JWT 鉴权 + user_id 级数据隔离 + 仓库/对话所有权校验

## 技术栈

| 层 | 选型 |
|------|------|
| 后端框架 | Go 1.22 + Gin |
| 数据库 | PostgreSQL 16 + pgvector |
| 缓存/队列 | Redis 7 |
| 代码解析 | Go: `go/parser` + `go/ast` |
| Embedding | Python gRPC Sidecar + BGE-M3 (1024维) |
| Rerank | BGE-Reranker (本地) |
| LLM | Claude API / DeepSeek API |
| 前端 | React 18 + Ant Design 5 + Vite |
| 部署 | Docker Compose 四服务编排 |

## 架构

```
Go Backend (Gin)
  ├── Auth Service      (JWT + GitHub OAuth)
  ├── Repo Service      (仓库 CRUD + Clone)
  ├── Indexer Service   (AST 解析 → FileNode / FuncNode / CallEdge / DepEdge)
  ├── Vector Store      (pgvector 写入 + 语义搜索 + 调用图扩展)
  ├── QA Service        (RAG 管线: embed → search → expand → rerank → LLM stream)
  ├── Task Service      (异步索引任务管理)
  └── Embedding Client  (gRPC → Python Sidecar)

Python Embedding Sidecar
  ├── BGE-M3 (1024-dim embedding)
  └── BGE-Reranker (cross-encoder scoring)

React Frontend (Ant Design)
  ├── 仓库列表 + 接入
  ├── 问答界面 (SSE 流式 + 引用高亮 + 置信度)
  └── 索引状态轮询
```

## 快速开始

### 前置条件

- Go 1.22+
- Docker & Docker Compose
- GitHub OAuth App (Settings → Developer settings → OAuth Apps)
- Claude 或 DeepSeek API Key

### 1. 配置

```bash
cp .env.example .env
# 编辑 .env:
#   GITHUB_CLIENT_ID=your_id
#   GITHUB_CLIENT_SECRET=your_secret
#   JWT_SECRET=生成一个随机字符串
#   LLM_API_KEY=your_api_key
```

### 2. 启动

```bash
docker compose up -d --build
# 打开 http://localhost:8080
```

### 3. 本地开发

```bash
# 启动基础设施
make db-up

# 启动后端
go run ./cmd/server

# 启动前端开发服务器
cd web && npm install && npm run dev
```

## 项目结构

```
.
├── cmd/server/main.go              # 入口，组装依赖
├── internal/
│   ├── domain/                     # 纯数据模型
│   ├── config/                     # 环境变量配置
│   ├── db/                         # PostgreSQL 连接 + Migration
│   ├── auth/                       # JWT 签发/校验 + Gin 中间件
│   ├── repo/                       # 仓库接入 (GitHub API + Clone)
│   ├── indexer/                    # Go AST 代码索引
│   ├── embedding/                  # gRPC Client → Python Sidecar
│   ├── vectorstore/                # pgvector 读写 + 混合检索
│   ├── qa/                         # RAG 编排 + LLM 流式调用
│   ├── task/                       # 异步任务管理
│   └── handler/                    # HTTP Handler (薄层)
├── proto/embedding.proto           # gRPC 定义
├── python-embedding/               # Python Embedding Sidecar
├── migrations/                     # 数据库 DDL
├── web/                            # React 前端
└── docker-compose.yml
```

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/github/callback` | GitHub OAuth → JWT |
| GET | `/api/repos` | 仓库列表 |
| POST | `/api/repos` | 接入仓库（启动索引） |
| GET | `/api/repos/:id` | 仓库详情 |
| DELETE | `/api/repos/:id` | 删除仓库 |
| GET | `/api/repos/:id/files` | 文件列表 |
| GET | `/api/repos/:id/graph` | 调用/依赖图 |
| POST | `/api/ask` | 代码问答（SSE 流） |
| GET | `/api/conversations` | 对话历史 |
| GET | `/api/conversations/:id` | 对话详情 |
| GET | `/api/tasks/:id` | 任务进度 |

### SSE 事件格式

```
event: chunk
data: {"text":"鉴权逻辑在","citations":[{"file":"auth/jwt.go","line":42,"score":0.89}]}

event: done
data: {"confidence":0.87,"tokens":1247,"conv_id":"xxx"}
```

## Make 命令

```bash
make dev        # 启动开发服务器
make build      # 编译二进制
make test       # 运行测试
make db-up      # 启动 PostgreSQL + Redis
make dc-up      # Docker Compose 全栈启动
make dc-down    # 停止所有服务
make proto      # 生成 gRPC 代码
```

## License

MIT
