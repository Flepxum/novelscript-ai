# 后端架构设计

## 1. 后端定位

Go 后端负责核心业务闭环：接收小说文本、切分章节、调用模型 provider、校验结构、保存版本、导出 YAML。前端不直接访问模型 provider，也不持有 API Key。

## 2. 技术栈

- Go：主要后端语言。
- Gin：HTTP 路由和中间件。
- SQLite：比赛阶段的轻量持久化。
- `database/sql`：数据库访问基础能力。
- `gopkg.in/yaml.v3`：YAML 序列化和反序列化。
- 标准 HTTP Client：按 OpenAI-compatible `chat/completions` 协议封装 AI 调用。

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
| `config` | 读取端口、数据库路径、Schema 路径、模型 base URL、Key 和模型名 |
| `handler` | HTTP 参数解析、响应封装、错误转换 |
| `service` | 项目、章节、生成、编辑、导出的业务编排 |
| `domain` | 项目、章节、角色、场景、剧本等核心结构 |
| `parser` | 规则章节识别、文本清洗、段落规整 |
| `ai` | OpenAI-compatible 调用、多 Agent 生成编排、提示词模板、结构化输出 |
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

`handler` 不直接访问数据库或模型 provider。`service` 是唯一的业务编排层，避免逻辑散落在 HTTP 层。

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
- `splitting`：确认章节边界和来源引用。
- `analyzing`：ChapterAnalysisAgent 逐章提炼事件、角色、地点和冲突。
- `outlining`：StoryBibleAgent 与 ScenePlannerAgent 建立故事圣经、幕结构和智能场数。
- `generating_scenes`：SceneExpansionAgent 逐场生成节拍、动作和对白。
- `validating`：校验 Schema、引用关系，必要时交给 ValidationRepairAgent。
- `exporting`：导出结构化 YAML。
- `succeeded`
- `failed`

前端通过 `GET /jobs/:id` 轮询进度。后续如果要提升体验，可以扩展为 Server-Sent Events。

## 8. 模型调用策略

- Key 从后端环境变量读取。
- 模型名从后端配置读取，不在代码中写死。
- 请求超时、重试次数、最大章节长度都通过配置控制。
- AI 输出先落到结构化 JSON，再转换成 YAML。
- `MODEL_AGENT_PIPELINE=multi_agent` 为默认生产模式：先逐章分析，再建立故事圣经，再规划场景卡，再逐场扩写，最后组装和校验。
- 章节规则切分失败时，`ChapterSegmentationAgent` 只返回段落边界，后端从原文重建章节正文。
- `JOB_MAX_PARALLEL` 控制章节分析和场景扩写的并发量，避免长篇小说只能串行处理。
- 如果 JSON 解析失败，进入 Malformed JSON 修复 Agent；如果 Schema 或引用校验失败，进入 ValidationRepairAgent。

## 9. 数据库设计

最小表结构：

- `projects`
- `source_documents`
- `chapters`
- `generation_jobs`
- `script_versions`
- `edit_logs`

每次生成和保存都创建新版本，不覆盖旧版本。这样 demo 时可以展示“AI 初稿 -> 人工编辑 -> 新版本”的完整过程。

当前比赛可运行版本先使用 `MemoryRepository`，原因是 demo 环境无需数据库初始化，评委拉取后能直接运行。代码已经把存储集中在 `repository` 模块，后续替换 SQLite 时只需要实现同一组保存、读取、版本查询方法，不需要改动 handler、service、parser、ai、validator 或前端。

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
- AI Client：使用本地 fixture 或测试替身做稳定测试。

