# 后端架构设计

## 1. 后端定位

Go 后端负责核心业务闭环：接收小说文本、切分章节、调用 OpenAI、校验结构、保存版本、导出 YAML。前端不直接访问 OpenAI，也不持有 API Key。

## 2. 技术栈

- Go：主要后端语言。
- Gin：HTTP 路由和中间件。
- SQLite：比赛阶段的轻量持久化。
- `database/sql`：数据库访问基础能力。
- `gopkg.in/yaml.v3`：YAML 序列化和反序列化。
- OpenAI Go SDK 或标准 HTTP Client：封装 AI 调用。

## 3. 目录结构

```text
backend/
  cmd/
    api/
      main.go
  internal/
    config/
    handler/
    service/
    domain/
    parser/
    ai/
    validator/
    exporter/
    repository/
    job/
  migrations/
  testdata/
```

## 4. 模块职责

| 模块 | 职责 |
| --- | --- |
| `config` | 读取端口、数据库路径、OpenAI Key、模型名 |
| `handler` | HTTP 参数解析、响应封装、错误转换 |
| `service` | 项目、章节、生成、编辑、导出的业务编排 |
| `domain` | 项目、章节、角色、场景、剧本等核心结构 |
| `parser` | 章节识别、文本清洗、段落规整 |
| `ai` | OpenAI Client、提示词模板、结构化输出 |
| `validator` | Schema 校验、引用一致性校验 |
| `exporter` | YAML 导出和文件命名 |
| `repository` | SQLite 读写 |
| `job` | 异步任务状态、进度和错误记录 |

## 5. 分层依赖

```text
handler -> service -> repository
handler -> service -> parser
handler -> service -> ai
handler -> service -> validator
handler -> service -> exporter
```

`handler` 不直接访问数据库或 OpenAI。`service` 是唯一的业务编排层，避免逻辑散落在 HTTP 层。

## 6. 核心领域对象

- `Project`：改编项目。
- `SourceDocument`：原小说文本。
- `Chapter`：切分后的章节。
- `GenerationJob`：一次 AI 生成任务。
- `ScriptDraft`：结构化剧本对象。
- `ScriptVersion`：一次保存或导出的版本。

这些对象对应数据库表，也对应前后端传输结构。

## 7. 异步任务设计

生成剧本可能耗时较长，因此 `POST /generate` 只创建任务并返回 `job_id`。

任务状态：

- `queued`
- `splitting`
- `analyzing`
- `outlining`
- `generating_scenes`
- `validating`
- `exporting`
- `succeeded`
- `failed`

前端通过 `GET /jobs/:id` 轮询进度。后续如果要提升体验，可以扩展为 Server-Sent Events。

## 8. OpenAI 调用策略

- Key 从 `OPENAI_API_KEY` 读取。
- 模型从 `OPENAI_MODEL` 读取，不在代码中写死。
- 请求超时、重试次数、最大章节长度都通过配置控制。
- AI 输出先落到结构化 JSON，再转换成 YAML。
- 如果 Schema 校验失败，只重试失败片段。

## 9. 数据库设计

最小表结构：

- `projects`
- `source_documents`
- `chapters`
- `generation_jobs`
- `script_versions`
- `edit_logs`

每次生成和保存都创建新版本，不覆盖旧版本。这样 demo 时可以展示“AI 初稿 -> 人工编辑 -> 新版本”的完整过程。

## 10. 错误处理

统一错误响应：

```json
{
  "error": {
    "code": "validation_failed",
    "message": "script schema validation failed",
    "details": [
      {
        "path": "scenes[0].chapter_refs",
        "message": "chapter ref 9 does not exist"
      }
    ]
  }
}
```

错误码保持稳定，前端只依赖 `code` 和 `details.path`。

## 11. 测试重点

- 章节切分：中文章节、英文 Chapter、无标题长文本。
- Schema 校验：缺字段、非法引用、空场景。
- YAML 导出：特殊字符、中文、换行对白。
- 生成任务：成功、失败、重试、取消。
- AI Client：使用 mock client 做稳定测试。

