# PR 提交说明

比赛规则要求持续交付和清晰 PR。建议不要把当前实现一次性塞进一个 PR，而是按下面顺序拆分提交。

## PR 2：初始化 Go 后端服务

### 功能描述

新增 Go + Gin API 骨架，提供健康检查、统一响应结构和配置读取。

### 实现思路

建立 `backend/cmd/api` 与 `backend/internal` 分层目录，配置从环境变量读取，HTTP 层统一封装成功和错误响应。

### 测试方式

- `cd backend && go test ./...`
- `go run ./cmd/api`
- 访问 `/api/v1/health`

## PR 3：小说导入与章节切分

### 功能描述

支持粘贴小说文本，自动识别中文章节和英文 Chapter 标题，少于 3 章返回校验错误。

### 实现思路

`parser` 模块负责换行清洗、章节标题识别和字数统计，`source` 接口保存章节切分结果。

### 测试方式

- `go test ./internal/parser`
- 使用 `backend/testdata/sample_novel.txt` 调用 `/api/v1/projects/:id/source`

## PR 4：YAML 导出与业务校验

### 功能描述

新增结构化剧本领域对象、YAML 序列化、编辑保存和引用关系校验。

### 实现思路

`domain` 定义 Schema 对应结构，`exporter` 负责 YAML 解析/导出，`validator` 检查章节、角色、幕和场景引用。

### 测试方式

- `go test ./internal/validator`
- 修改 YAML 中的角色引用，确认保存接口返回 `validation_failed`

## PR 5：剧本生成编排与异步任务

### 功能描述

新增生成编排层和生成任务状态流转，在模型配置接入前也能稳定生成剧本 YAML。

### 实现思路

通过 `internal/ai` 生成编排层集中剧本初稿生成逻辑；任务状态从 queued 流转到 succeeded，成功后保存脚本版本。

### 测试方式

- 调用 `/generate`
- 轮询 `/jobs/:id`
- 读取 `/script` 获得 YAML

## PR 6：模型 provider

### 功能描述

新增模型 provider，配置 Key 和模型名后可以真实生成。

### 实现思路

后端读取模型配置，调用模型接口并解析 JSON 输出，复用同一套校验。

### 测试方式

- 配置模型环境变量
- 使用短篇 3 章文本发起生成
- 检查输出能通过保存接口校验

## PR 7：React 工作台

### 功能描述

新增前端工作台，覆盖小说导入、章节展示、生成进度、YAML 编辑、剧本预览和导出。

### 实现思路

使用 React + Vite + TypeScript 搭建三栏布局，API client 集中封装后端请求，编辑器使用 YAML 文本区和结构化预览联动。

### 测试方式

- `cd frontend && npm install && npm run build`
- 前后端同时启动，按 README 演示流程跑通

## PR 8：局部重写、版本和 Demo 文档

### 功能描述

支持单场景重写、版本列表展示、Schema 查看，并补充 README、demo 录制脚本和提交说明。

### 实现思路

重写接口读取最新剧本版本，仅更新目标场景后重新校验和保存；文档补齐依赖、原创说明和演示步骤。

### 测试方式

- 生成剧本后选择 `s01` 重写
- 确认版本列表新增版本
- 导出 YAML 并检查根结构完整
