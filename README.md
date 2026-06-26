# ScholarFlow Server 📚

ScholarFlow Server 是 ScholarFlow 的后端服务。它负责接收本地 PDF 上传，保存原始文件，异步解析论文，并在配置 LLM 后生成可溯源的论文阅读卡片。

当前实现重点放在服务端链路：API、数据库 schema、异步任务、GROBID 解析、Reader 输出校验。Obsidian 插件会在后续阶段通过 API 同步结构化结果，不直接保存原始 PDF。

## 功能✅

- 本地 PDF 上传，限制默认 50 MiB
- 原始 PDF 存入 S3 兼容对象存储，本地使用 MinIO
- PostgreSQL 保存论文、资产、任务、作者、章节、参考文献、图表和证据数据
- Redis + asynq 驱动异步处理任务
- GROBID 解析 PDF，提取标题、摘要、作者、章节、参考文献、DOI、年份、图表和表格标题
- 保存 GROBID TEI XML，便于后续追溯和二次处理
- 可选 OpenAI 兼容 Reader，生成 JSON paper card 和 claim-level evidence
- 可选 arXiv 自动获取，按分类每日抓取新论文并走完整处理流程
- 失败任务支持按阶段重试，过期失败任务支持定时清理

任务状态流转：

```text
queued -> processing -> parsed -> reading -> completed
                              \-> failed
```

如果没有配置 Reader，任务会停在 `parsed`。配置 `OPENAI_BASE_URL` 和 `OPENAI_API_KEY` 后，解析成功的任务会继续进入 `reading`，成功后变为 `completed`。

## 目录结构 🗂️

```text
.
├── cmd/
│   ├── server/        # HTTP API 入口
│   └── worker/        # asynq worker 入口
├── docs/              # API 和设计说明
├── internal/
│   ├── config/        # 环境变量配置
│   ├── db/            # sqlc 生成代码和数据库连接
│   ├── httpapi/       # HTTP 路由和 handler
│   ├── jobs/          # 异步任务、解析流水线、Reader 流水线、清理任务
│   ├── papers/        # 论文领域服务和查询逻辑
│   ├── parser/        # parser 接口和 GROBID 适配器
│   ├── reader/        # LLM Reader、schema 校验、prompt
│   └── storage/       # MinIO/S3 存储适配器
├── migrations/        # PostgreSQL migrations
├── queries/           # sqlc 查询
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── sqlc.yaml
```

## 快速启动 🚀

准备本地配置：

```bash
cp .env.example .env
```

启动 PostgreSQL、Redis、MinIO 和 GROBID：

```bash
docker compose up -d postgres redis minio grobid
```

首次启动前需要执行数据库迁移。当前服务不会自动跑 migrations：

```bash
export DATABASE_URL='postgres://scholarflow:scholarflow@localhost:5432/scholarflow?sslmode=disable'
go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations postgres "$DATABASE_URL" up
```

启动 API 和 worker：

```bash
docker compose up -d --build api worker
```

检查服务：

```bash
curl http://localhost:8080/healthz
```

预期返回：

```text
ok
```

查看日志：

```bash
docker compose logs -f api worker
```

## 上传一篇论文 🧪

准备一个本地 PDF，然后调用上传接口：

```bash
export PDF_PATH="/absolute/path/to/paper.pdf"

curl -i -X POST http://localhost:8080/v1/uploads/papers \
  -F "file=@${PDF_PATH};type=application/pdf"
```

成功时返回 `202 Accepted`：

```json
{"paper_id":"...","job_id":"..."}
```

查询任务状态：

```bash
curl -s http://localhost:8080/v1/jobs/$JOB_ID | python3 -m json.tool
```

查询论文详情：

```bash
curl -s http://localhost:8080/v1/papers/$PAPER_ID | python3 -m json.tool
```

`GET /v1/papers/{id}` 会返回论文元数据、作者、章节、参考文献、图表/表格标题，以及最新的 `card`（如果 Reader 已生成）。

## API 一览 🔌

| Method | Path | 说明 |
| --- | --- | --- |
| `GET` | `/healthz` | 健康检查 |
| `POST` | `/v1/uploads/papers` | 上传 PDF，字段名为 `file` |
| `GET` | `/v1/jobs/{id}` | 查询处理任务状态 |
| `GET` | `/v1/papers/{id}` | 查询论文详情 |
| `POST` | `/v1/jobs/{id}/retry` | 重试失败任务 |
| `POST` | `/v1/harvest/arxiv` | 手动触发一次 arXiv 抓取，可选 body 覆盖分类 |

更多响应示例见 `docs/api.md`。

## Reader 配置 🤖

Reader 默认关闭。只要 `OPENAI_BASE_URL` 或 `OPENAI_API_KEY` 为空，worker 会在解析完成后停在 `parsed`。

在 `.env` 中配置：

```bash
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
OPENAI_MAX_INPUT_CHARS=48000
OPENAI_TIMEOUT_SECONDS=120
OPENAI_API_STYLE=chat
OPENAI_RESPONSE_FORMAT=json_schema
OPENAI_SYSTEM_PROMPT_PATH=/app/prompts/system_zh.md
READ_MAX_RETRY=3
JOB_FAILED_RETENTION_DAYS=7
JOB_CLEANUP_CRON=@daily
```

说明：

- `OPENAI_API_STYLE=chat` 使用 `/chat/completions`
- `OPENAI_API_STYLE=responses` 使用 `/responses`，只在提供商支持 OpenAI Responses API 时使用
- `OPENAI_RESPONSE_FORMAT=json_schema` 会请求结构化 JSON 输出
- 如果你的提供商不支持 JSON schema，可以改成 `json_object`
- `OPENAI_SYSTEM_PROMPT_PATH` 是容器内路径，不是宿主机路径
- docker-compose 会把 `internal/reader/prompts` 只读挂载到容器内 `/app/prompts`

修改 Reader 配置后重启服务：

```bash
docker compose up -d --build api worker
```

## arXiv 自动获取 📡

worker 会按订阅的 arXiv 分类抓取最新提交的论文，下载 PDF 后走与本地上传**完全相同**的处理流程（GROBID 解析 →（如配置了 Reader）生成 paper card）。抓取到的论文只是 `source_type` 为 `arxiv`、并带有 `source_id`（arXiv id），其余在 API 中与上传论文无差别。

有两种触发方式：

- **自动（计划任务）**：开启后在内置 `asynq.Scheduler` 上注册 `arxiv:harvest` 计划任务，按 `ARXIV_HARVEST_CRON` 在**指定时间**运行。
- **手动（API）**：`POST /v1/harvest/arxiv` 立即触发一次抓取，可在 body 中覆盖分类（见下方「手动触发」）。只要配置了分类（或开启了功能），worker 就会注册处理器，因此即便关闭了计划任务也能手动触发。

在 `.env` 中配置：

```bash
ARXIV_HARVEST_ENABLED=true
ARXIV_HARVEST_CATEGORIES=cs.CL,cs.AI
ARXIV_HARVEST_CRON=0 8 * * *
ARXIV_HARVEST_TIMEZONE=Asia/Shanghai
ARXIV_HARVEST_MAX_RESULTS=50
ARXIV_API_BASE_URL=http://export.arxiv.org/api/query
ARXIV_REQUEST_DELAY_SECONDS=3
```

说明：

- `ARXIV_HARVEST_ENABLED=false` 时不注册**计划任务**；但只要 `ARXIV_HARVEST_CATEGORIES` 非空，处理器仍会注册，可通过 API 手动触发（即「仅手动」模式）
- `ARXIV_HARVEST_CATEGORIES` 为逗号分隔的 arXiv 分类代码，留空则不抓取
- **指定运行时间**：`ARXIV_HARVEST_CRON` 用 cron 表达式指定时间（如 `0 8 * * *` 为每天 08:00）；`ARXIV_HARVEST_TIMEZONE` 指定时区，**留空则使用部署系统本地时区（`TZ`）**。asynq 默认按 UTC 触发，本项目改为按该时区触发，使「指定时间」与部署机器一致
- 每个分类按提交时间倒序取前 `ARXIV_HARVEST_MAX_RESULTS` 篇；若某分类单日新增超过该上限，超出的尾部会被漏掉（可调大该值）
- 抓取是**尽力而为**的：单篇下载/解析失败会记录日志并跳过，不影响其它论文；通过 `source_id` 去重，因此重跑或漏跑某天都是幂等的
- 下载有大小上限（复用 `MAX_UPLOAD_BYTES`），并会校验 PDF 文件头，HTML 错误页不会被入库
- `ARXIV_REQUEST_DELAY_SECONDS` 同时用于两处礼貌性延迟：每次分类元数据查询之间，以及每篇 PDF 下载之间
- ⚠️ 若已配置 Reader，按分类抓取可能产生**可观的 LLM 调用成本**（与上传走同一路径）；Reader 默认关闭，开启前请评估

**分类代码在哪里找：**

- 官方分类总表（推荐）：<https://arxiv.org/category_taxonomy>
- 常用示例：`cs.CL`（计算语言学/NLP）、`cs.AI`（人工智能）、`cs.LG`（机器学习）、`cs.CV`（计算机视觉）、`stat.ML`（统计机器学习）
- 也可在 arXiv 任意分类列表页 URL 中看到代码，如 `https://arxiv.org/list/cs.CL/recent`

**手动触发：**

```bash
# 使用已配置的分类立即抓取一次
curl -i -X POST http://localhost:8080/v1/harvest/arxiv

# 本次抓取覆盖分类（仅影响这一次，不改动配置）
curl -i -X POST http://localhost:8080/v1/harvest/arxiv \
  -H 'Content-Type: application/json' \
  -d '{"categories":["cs.CL","cs.CV"]}'
```

返回 `202 Accepted` 与 `{"task_id":"..."}`；实际抓取由 worker 异步执行，结果通过 `GET /v1/papers` 查看。

修改配置后重启 worker：

```bash
docker compose up -d --build worker
```

## 配置项 📋

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | API 监听地址 |
| `DATABASE_URL` | localhost PostgreSQL | PostgreSQL 连接串 |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址 |
| `MINIO_ENDPOINT` | `localhost:9000` | MinIO/S3 endpoint |
| `MINIO_ACCESS_KEY` | `scholarflow` | MinIO access key |
| `MINIO_SECRET_KEY` | `scholarflow-secret` | MinIO secret key |
| `MINIO_BUCKET` | `scholarflow` | 对象存储 bucket |
| `MINIO_USE_SSL` | `false` | 是否使用 HTTPS 连接对象存储 |
| `GROBID_URL` | `http://localhost:8070` | GROBID 服务地址 |
| `MAX_UPLOAD_BYTES` | `52428800` | 上传大小限制 |
| `OPENAI_BASE_URL` | 空 | OpenAI 兼容 API base URL，留空禁用 Reader |
| `OPENAI_API_KEY` | 空 | Reader API key，留空禁用 Reader |
| `OPENAI_MODEL` | `gpt-4o-mini` | Reader 使用的模型名 |
| `OPENAI_MAX_INPUT_CHARS` | `48000` | Reader 输入截断长度 |
| `OPENAI_TIMEOUT_SECONDS` | `120` | Reader 请求超时 |
| `OPENAI_API_STYLE` | `chat` | `chat` 或 `responses` |
| `OPENAI_RESPONSE_FORMAT` | `json_schema` | `json_schema` 或 `json_object` |
| `OPENAI_SYSTEM_PROMPT_PATH` | 空 | 容器内系统提示词路径 |
| `READ_MAX_RETRY` | `3` | Reader 任务最大重试次数 |
| `JOB_FAILED_RETENTION_DAYS` | `7` | 失败任务保留天数 |
| `JOB_CLEANUP_CRON` | `@daily` | 失败任务清理计划 |
| `ARXIV_HARVEST_ENABLED` | `false` | 是否开启 arXiv 每日自动获取 |
| `ARXIV_HARVEST_CATEGORIES` | 空 | 逗号分隔的 arXiv 分类代码，如 `cs.CL,cs.AI` |
| `ARXIV_HARVEST_CRON` | `@daily` | 抓取计划（asynq cron 表达式） |
| `ARXIV_HARVEST_MAX_RESULTS` | `50` | 每个分类每次抓取的上限 |
| `ARXIV_API_BASE_URL` | `http://export.arxiv.org/api/query` | arXiv Query API 地址 |
| `ARXIV_REQUEST_DELAY_SECONDS` | `3` | 请求间礼貌性延迟（查询与下载） |
| `ARXIV_HARVEST_TIMEZONE` | 空 | 计划任务时区，留空用部署系统本地时区（`TZ`） |

## TODO ☑️

- [x] arxiv 自动化论文获取
- [ ] LLM 翻译接口

## 致谢 / Acknowledgements 🙏

本项目依赖以下开源工具：

- [GROBID](https://github.com/kermitt2/grobid) — scholarly PDF 解析为 TEI XML
- [Poppler](https://poppler.freedesktop.org/) (`pdftoppm`) — 渲染 PDF 页面以裁剪图表/系统架构图
- [asynq](https://github.com/hibiken/asynq) — Redis 异步任务队列
- [sqlc](https://sqlc.dev/) — 从 SQL 生成类型安全的 Go 数据访问代码
- [goose](https://github.com/pressly/goose) — 数据库迁移
