# CodeBase Copilot Platform — 简历项目包装方案

> 面向 Go 后端 / AI Agent 实习方向

---

## 一、简历可直接使用版本（推荐）

### 项目名称
**CodeBase Copilot — 面向 Git 仓库的 AI 代码问答平台**

### 一句话简介
接入 GitHub/Gitee 仓库后自动解析代码结构、构建向量索引，支持基于 RAG 的自然语言代码问答（流式输出 + 引用溯源）。

### 技术栈
Go 1.22 / Gin / PostgreSQL + pgvector / Redis / gRPC / Protobuf / Python Sidecar (BGE-M3 + BGE-Reranker) / Docker Compose / React + Ant Design

### 项目描述（简历正文，4 条 bullet）

- **设计并实现完整 RAG 管线**：问题向量化 → pgvector 语义检索 Top-50 → 调用图 ±1 跳扩展 → BGE-Reranker 精排 Top-10 → LLM 流式生成，SSE 实时推送回答片段、引用文件及置信度
- **基于 Go AST 构建代码结构化索引**：用 `go/parser`+`go/ast` 遍历语法树提取函数节点、调用边和依赖边，构建四层索引（文件→函数→调用关系→依赖图）；集成 universal-ctags 支持 15+ 语言
- **Go + Python gRPC 跨语言架构**：Go 主服务通过 gRPC 调用 Python Sidecar（BGE-M3 向量化 + BGE-Reranker 精排），模型推理与业务逻辑解耦；批量 embedding 减少 RPC 往返
- **生产级工程实践**：JWT 鉴权 + 多租户数据隔离 + 仓库所有权校验；Redis 滑动窗口限流（内存令牌桶降级）；Prometheus 指标采集 + 结构化日志；Docker Compose 五服务编排一键部署

---

## 二、精简版（适合简历空间有限时，2~3 条）

- **面向 Git 仓库的 AI 代码问答平台**，实现 RAG 全流程：Go AST 代码解析 → pgvector 向量检索 → 调用图扩展 → Reranker 精排 → LLM 流式生成，SSE 实时推送引用溯源与置信度
- **Go + Python gRPC 跨语言架构**，Go 主服务通过 gRPC 调用 Python Sidecar 完成 BGE-M3 向量化与 BGE-Reranker 精排；集成 universal-ctags 支持 15+ 语言代码解析
- **生产级工程能力**：JWT 多租户隔离、Redis 滑动窗口限流（内存令牌桶降级）、Prometheus 监控、结构化日志、优雅关停；Docker Compose 五服务编排一键部署

---

## 三、技术亮点深度分析（面试时展开讲）

### 1. RAG 管线设计（最核心亮点，Agent 方向必问）

```
用户问题
  │
  ▼
Embed (BGE-M3 1024维)
  │
  ▼
pgvector 语义检索 Top-50          ← 余弦距离排序
  │
  ▼
调用图 ±1 跳扩展 (GraphExpand)    ← 基于 call_edges 表 JOIN
  │
  ▼
去重 (by node ID)
  │
  ▼
Reranker 精排 Top-10 (BGE-Reranker) ← Cross-Encoder 交叉编码
  │
  ▼
组装 Context + 注入对话历史 (token 截断)
  │
  ▼
LLM 流式生成 → SSE 推送前端
```

**为什么这个设计好：**
- 不是简单的"向量检索 → LLM"，而是**多阶段检索 + 图扩展 + 精排**
- GraphExpand 是亮点：语义检索只找到"相似"的代码，但函数 A 调用了函数 B，用户问 A 时可能也需要 B 的上下文 → 通过 call_edges 表做 ±1 跳扩展
- Reranker 解决了向量检索"召回高但精度不足"的问题：先宽召回 50 个，再用 Cross-Encoder 精排到 10 个
- 置信度计算：用 rerank 分数均值做 sigmoid 归一化，给用户一个可量化的信任指标

### 2. Go AST 代码索引（Go 后端方向核心亮点）

```go
// 核心流程：parser.ParseDir → ast.Inspect → 提取 FuncDecl → 构建 funcMap → 解析 CallExpr → 回填调用边
```

**技术细节：**
- 使用标准库 `go/parser` + `go/ast`，零外部依赖解析 Go 源码
- 提取四类信息：文件节点、函数/方法节点（含签名、源码、行号）、调用边（caller→callee）、依赖边（import 关系）
- `funcMap` 做 name→ID 映射，二次遍历解析调用边的 callee（先收集所有函数，再回填调用关系）
- 支持方法接收者（`*ast.StarExpr` / `*ast.Ident`）的类型提取
- `buildSignature` 手动重建函数签名字符串（包括参数类型、返回值类型），用于 embedding 输入

### 3. gRPC 跨语言通信

```
Go Backend  ──gRPC──>  Python Embedding Sidecar
  │                        │
  │ Embed(texts)           │ BGE-M3 (1024维)
  │ Rerank(query, docs)    │ BGE-Reranker (CrossEncoder)
  ▼                        ▼
```

- Protobuf 定义服务接口（`embedding.proto`）
- Go 端用 `grpc.DialContext` + insecure credentials 连接
- Python 端用 `grpc_tools` 生成 stub，`SentenceTransformer` 加载模型
- 批量 embedding（batch_size=4）减少 gRPC 往返

### 4. 生产级中间件链

| 中间件 | 实现方式 | 面试讲点 |
|--------|---------|---------|
| 限流 | Redis 滑动窗口 + 内存令牌桶降级 | 分布式限流 vs 单机限流，fail-open 策略 |
| 鉴权 | JWT + GitHub/Gitee OAuth | 多租户隔离，仓库所有权校验 |
| 监控 | Prometheus CounterVec + HistogramVec | 指标基数控制（normalizePath） |
| 日志 | slog 结构化 JSON + RequestID | 链路追踪，日志分级 |
| 安全 | BodyLimit + CORS + Security Headers | 防护纵深 |

### 5. 数据库设计

- `index_nodes`：核心表，`embedding vector(1024)` 用 pgvector 存储
- `call_edges` / `dep_edges`：调用图和依赖图，FK 关联 index_nodes
- 重建索引时先删 edges 再删 nodes（FK 约束顺序），事务保证一致性
- `vector <=> query_vec` 余弦距离排序，`IS NOT NULL` 过滤无向量的文件节点

---

## 四、面试高频问题 & 回答要点

### Q1: 你的 RAG 和直接调 LLM 有什么区别？
**答：** 直接调 LLM 有两个问题：一是 LLM 不可能记住整个仓库的代码（上下文窗口有限），二是容易幻觉。我的方案是先用向量检索找到最相关的代码片段，再把代码作为 context 喂给 LLM，让 LLM "看着代码回答"。为了提升检索质量，我做了三步优化：先用 BGE-M3 做语义检索宽召回 50 个，再通过调用图扩展补充上下文相关的函数，最后用 BGE-Reranker 做 Cross-Encoder 精排到 10 个。Reranker 比向量检索更准，因为它能同时看到 query 和 document 做交叉编码，但计算成本更高，所以放在最后一步只排少量数据。

### Q2: 为什么要做调用图扩展？
**答：** 语义检索只能找到和问题"文本相似"的代码，但代码理解需要上下文。比如用户问"鉴权逻辑在哪"，语义检索可能找到 `RequireAuth` 函数，但这个函数内部调用了 `ValidateToken`，用户可能也需要看到 `ValidateToken` 的实现。所以我用 call_edges 表做 ±1 跳扩展，把被调用函数也加到候选集里，但给一个较低的分值（0.3）避免噪声。最终由 Reranker 决定它们是否真的相关。

### Q3: 为什么用 gRPC 而不是 HTTP 调 Python？
**答：** 三个原因：一是 gRPC 用 Protobuf 二进制序列化，比 JSON 更高效，尤其是传 1024 维 float 向量时差距明显；二是 gRPC 天然支持流式，未来可以扩展为流式 embedding；三是强类型接口，Protobuf 定义就是契约，Go 和 Python 各自生成 stub，不容易出参数错误。实际实现中我用了批量 embedding（batch_size=4）来减少 gRPC 调用次数。

### Q4: Go AST 解析具体做了什么？
**答：** 我用标准库 `go/parser` 把 Go 源码解析成 AST，然后用 `ast.Inspect` 遍历语法树。遇到 `*ast.FuncDecl` 节点就提取函数名、接收者类型（区分普通函数和方法）、参数和返回值类型重建签名，再从源文件中按行号截取函数体代码。同时遍历函数体内的 `*ast.CallExpr` 提取调用关系。所有函数收集完后，用 `funcMap`（函数全名→节点 ID）回填调用边的 callee ID。最后把这些节点和边批量写入 PostgreSQL，函数节点还会额外生成 embedding 存入 pgvector。

### Q5: 限流怎么做的？为什么有降级？
**答：** 我实现了两种限流：Redis 滑动窗口用于多实例部署（限流状态共享），内存令牌桶用于单机或 Redis 不可用时。启动时如果 Redis 连接成功就启用分布式限流，失败就 log warning 然后用内存限流，不让服务起不来。另外对 LLM 接口做了更严格的限流（5 req/s vs 普通 API 100 req/s），因为 LLM 调用成本高、耗时长。Redis 出错时采用 fail-open 策略（放行请求），避免限流组件故障导致服务不可用。

### Q6: 多租户隔离怎么保证的？
**答：** 三层：第一层 JWT 鉴权，每个请求验证 token 提取 user_id；第二层所有数据查询都带 user_id 条件（repos、conversations 表都有 user_id 外键）；第三层在关键操作前做所有权校验，比如 Ask 接口在执行 RAG 前先查 `repos.user_id` 是否匹配当前用户，不匹配直接返回 403。

### Q7: 如果仓库很大（几万个函数），索引会很慢怎么办？
**答：** 目前我的 embedding 是 batch_size=4 批量调用的，对大仓库确实会慢。优化方向有几个：一是增大 batch_size（BGE-M3 支持 32+）；二是引入任务队列（目前是同步触发的异步任务，但没有消息队列），可以用 Redis List 或专门的消息队列做真正的分布式任务调度；三是增量索引，只对 git diff 涉及的文件重新解析和 embedding，而不是全量重建。

---

## 五、简历优化建议

### 可以再加分的点（如果有时间做）：
1. **加上量化数据**："索引 XX 个函数节点，检索延迟 < XXms，问答端到端延迟 < XXs" — 用真实仓库（如某个中等规模开源项目）跑一下 benchmarks
2. **增量索引**：目前是全量重建，加一个 git diff 增量更新逻辑会大幅加分
3. **并发 embedding**：目前 batch 是串行的，可以用 `errgroup` 做并发批量 embedding
4. **测试覆盖**：有测试文件但覆盖率未知，补几个核心逻辑的单元测试（RAG pipeline、AST parser）
5. **上线演示**：如果能部署一个公网可访问的 demo 链接放简历上，效果非常好

### 面试时的讲述策略：
- **Go 后端岗**：重点讲 AST 解析、gRPC、中间件链（限流/鉴权/监控）、数据库设计、事务一致性
- **AI Agent 岗**：重点讲 RAG 管线设计（多阶段检索 + 图扩展 + 精排）、置信度计算、SSE 流式输出、对话历史 token 截断
- **通用策略**：先讲架构全景（30 秒），再按面试官兴趣深入某个模块

### 简历排版建议：
- 项目放在"项目经历"最前面（最重要的项目）
- 技术栈关键词加粗，方便 HR 和 ATS 扫描
- bullet 点控制在 4~5 条，每条不超过 2 行
- 如果有 GitHub 链接，放在项目名称旁边
