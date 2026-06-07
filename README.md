# NovelScript AI

AI 小说转剧本工具。它可以把 3 个章节以上的小说文本自动整理成可编辑、可追溯、可导出的结构化剧本初稿，输出格式为 YAML。

## 已完成功能

- 小说导入：支持粘贴文本和上传 `.txt`，自动识别中文章节与英文 Chapter 标题。
- 章节约束：少于 3 个章节会被后端拒绝，并返回可展示的校验错误。
- 剧本生成：支持异步任务、进度轮询和 OpenAI-compatible LLM 真实生成。
- YAML 输出：生成结果包含 `project/source/world/characters/acts/scenes/continuity/revision` 根结构。
- 校验保存：编辑后的 YAML 会先解析，再做引用关系和业务规则校验，成功后生成新版本。
- 局部重写：可选择单场景并根据指令重写，重写后自动更新 YAML 和版本。
- Schema 文档：提供人类可读文档和机器可读 JSON Schema。
- 前端工作台：三栏布局，覆盖导入、生成、编辑、预览、场景检查、复制和下载。

## 技术栈

- 前端：React + Vite + TypeScript + lucide-react + yaml
- 后端：Go + Gin + `gopkg.in/yaml.v3`
- AI：后端通过 OpenAI-compatible `chat/completions` 调用真实模型
- 存储：比赛演示版使用内存仓库；代码保留 repository 边界，便于后续替换为 SQLite
- 输出：YAML

## 启动与构建

建议使用两个终端分别启动后端和前端。先启动后端，再启动前端。

### 1. 准备依赖

后端依赖：

```bash
cd backend
go mod tidy
```

前端依赖：

```bash
cd frontend
npm install
```

### 2. 启动后端开发服务

macOS / Linux / Git Bash：

```bash
cd backend
go run ./cmd/api
```

Windows PowerShell：

```powershell
cd backend
go run .\cmd\api
```

默认后端地址：

```text
http://localhost:8080
```

健康检查：

```bash
curl http://localhost:8080/api/v1/health
```

Windows PowerShell 也可以使用：

```powershell
Invoke-RestMethod http://localhost:8080/api/v1/health
```

### 3. 启动前端开发服务

```bash
cd frontend
npm run dev
```

默认前端地址：

```text
http://localhost:5173
```

前端开发服务会把 `/api` 请求代理到 `http://localhost:8080`。

### 4. 后端构建

Windows：

```powershell
cd backend
go build -o .\bin\api.exe .\cmd\api
```

macOS / Linux：

```bash
cd backend
go build -o ./bin/api ./cmd/api
```

运行构建后的后端：

Windows：

```powershell
cd backend
.\bin\api.exe
```

macOS / Linux：

```bash
cd backend
./bin/api
```

### 5. 前端构建

```bash
cd frontend
npm run build
```

构建产物位于：

```text
frontend/dist/
```

本地预览生产构建：

```bash
cd frontend
npm run preview
```

### 6. 测试命令

后端测试：

```bash
cd backend
go test ./...
```

前端类型检查和生产构建：

```bash
cd frontend
npm run build
```

### 7. 一次完整本地演示流程

1. 终端 A：

```bash
cd backend
go run ./cmd/api
```

2. 终端 B：

```bash
cd frontend
npm run dev
```

3. 打开：

```text
http://localhost:5173
```

4. 在页面点击“示例” -> “切分” -> “生成”，查看 YAML、剧本预览、场景详情、局部重写和版本恢复。

## 环境配置

配置文件内容不提交。`.env`、`.env.*` 已被 `.gitignore` 忽略。

后端启动时会自动读取本地 `.env` 文件。推荐把后端配置写在 `backend/.env`，也支持仓库根目录 `.env`。如果同一个 key 同时存在于进程环境变量和 `.env`，进程环境变量优先生效。

Windows PowerShell 临时设置示例：

```powershell
$env:PORT="8080"
$env:SCRIPT_SCHEMA_PATH="../schemas/script.schema.json"
go run .\cmd\api
```

macOS / Linux 临时设置示例：

```bash
PORT=8080 SCRIPT_SCHEMA_PATH=../schemas/script.schema.json go run ./cmd/api
```

后端启动时会输出基础配置、LLM 配置状态和一次 LLM 连通性检查结果。连通性检查使用 OpenAI-compatible 接口约定，请将 `MODEL_BASE_URL` 配置为模型服务根路径，例如包含 `/v1` 的 base URL；启动检查会请求 `{MODEL_BASE_URL}/models`。检查失败只写入日志，不会阻塞 API 服务启动。

最小真实模型配置：

```text
MODEL_BASE_URL=<your-openai-compatible-base-url>
MODEL_API_KEY=<your-api-key>
MODEL_NAME=<your-model-name>
```

真实配置值只放在本地环境变量或本地配置文件中，不提交到仓库。

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

模型 provider、Key、模型名和结构化输出策略均由后端环境变量配置，前端不会直接接触模型配置。缺少 `MODEL_BASE_URL`、`MODEL_API_KEY` 或 `MODEL_NAME` 时，生成任务会失败并提示缺失项。

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
