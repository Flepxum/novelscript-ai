# NovelScript AI

AI 小说转剧本工具。它可以把 3 个章节以上的小说文本自动整理成可编辑、可追溯、可导出的结构化剧本初稿，输出格式为 YAML。

## 已完成功能

- 小说导入：支持粘贴文本和上传 `.txt`，自动识别中文章节与英文 Chapter 标题。
- 章节约束：少于 3 个章节会被后端拒绝，并返回可展示的校验错误。
- 剧本生成：支持异步任务、进度轮询和可替换的后端生成编排层。
- YAML 输出：生成结果包含 `project/source/world/characters/acts/scenes/continuity/revision` 根结构。
- 校验保存：编辑后的 YAML 会先解析，再做引用关系和业务规则校验，成功后生成新版本。
- 局部重写：可选择单场景并根据指令重写，重写后自动更新 YAML 和版本。
- Schema 文档：提供人类可读文档和机器可读 JSON Schema。
- 前端工作台：三栏布局，覆盖导入、生成、编辑、预览、场景检查、复制和下载。

## 技术栈

- 前端：React + Vite + TypeScript + lucide-react + yaml
- 后端：Go + Gin + `gopkg.in/yaml.v3`
- AI：后端生成编排层，模型配置由后续 PR 接入
- 存储：比赛演示版使用内存仓库；代码保留 repository 边界，便于后续替换为 SQLite
- 输出：YAML

## 快速启动

启动后端：

```bash
cd backend
go mod tidy
go run ./cmd/api
```

启动前端：

```bash
cd frontend
npm install
npm run dev
```

默认访问：

- 前端：`http://localhost:5173`
- 后端健康检查：`http://localhost:8080/api/v1/health`

## 环境配置

```bash
cd backend
copy .env.example .env
```

可配置项：

```text
APP_ENV=development
PORT=8080
API_BASE_PATH=/api/v1
PUBLIC_BASE_URL=http://localhost:8080
LOG_LEVEL=info
REQUEST_BODY_LIMIT_MB=20
CORS_ALLOWED_ORIGINS=http://localhost:5173
REPOSITORY_TYPE=memory
SQLITE_PATH=../data/novelscript.db
DATA_DIR=../data
EXPORT_DIR=../data/exports
MIN_CHAPTER_COUNT=3
MAX_CHAPTER_COUNT=80
MAX_CHAPTER_CHARS=20000
JOB_STEP_DELAY_MS=180
JOB_TIMEOUT_SECONDS=180
JOB_MAX_PARALLEL=2
SCRIPT_SCHEMA_PATH=../schemas/script.schema.json
MODEL_PROVIDER=
MODEL_BASE_URL=
MODEL_API_KEY=
MODEL_ORG_ID=
MODEL_PROJECT_ID=
MODEL_NAME=
MODEL_TIMEOUT_SECONDS=120
MODEL_MAX_RETRIES=2
MODEL_MAX_INPUT_CHARS=24000
MODEL_MAX_OUTPUT_TOKENS=6000
MODEL_TEMPERATURE=0.4
MODEL_STRUCTURE_MODE=json_schema
MODEL_PROMPT_VERSION=script-draft-v1
```

模型 provider、Key、模型名和结构化输出策略将在后续 PR 中接入，前端不会直接接触模型配置。

## 演示流程

1. 打开前端工作台。
2. 点击“示例”，载入内置 3 章小说。
3. 点击“切分”，查看章节识别结果。
4. 点击“生成”，等待任务进度到 100%。
5. 在分屏视图查看 YAML 和剧本预览。
6. 选择一个场景，输入重写指令并点击“重写所选场景”。
7. 点击“保存”“复制”或“导出”，获得可编辑 YAML 初稿。

## 文档

- [系统架构设计](docs/architecture.md)
- [前端架构设计](docs/frontend-architecture.md)
- [后端架构设计](docs/backend-architecture.md)
- [API 设计](docs/api-design.md)
- [AI 生成流水线](docs/ai-pipeline.md)
- [YAML Schema 设计](docs/yaml-schema.md)
- [开发路线与 PR 规划](docs/development-roadmap.md)
- [Demo 录制脚本](docs/demo-script.md)
- [PR 提交说明](docs/pr-submission-guide.md)

## 目录

- `frontend/`：React 工作台、YAML 编辑、剧本预览、导出界面
- `backend/`：Go API、章节解析、AI provider、校验、YAML 导出
- `backend/testdata/`：演示小说和示例 YAML
- `docs/`：架构、Schema、提交和演示文档
- `schemas/`：机器可读的结构化剧本 Schema

## 第三方依赖与原创说明

第三方依赖列在 `backend/go.mod` 和 `frontend/package.json` 中。原创实现包括章节切分规则、剧本生成编排层、YAML 业务校验、局部重写流程、API 编排和前端工作台交互。

## Demo

- 视频链接：待补充，录制提纲见 [docs/demo-script.md](docs/demo-script.md)。
