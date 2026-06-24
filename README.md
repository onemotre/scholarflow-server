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

## TODO ☑️

- [ ] arxiv 自动化论文获取
- [ ] LLM 翻译接口
