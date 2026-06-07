# 系统架构设计

## 1. 项目目标

把小说改编流程拆成“导入 - 分析 - 生成 - 校验 - 编辑 - 导出”六步，降低改编门槛，让作者快速拿到一份可继续修改的剧本初稿。

## 2. 设计原则

- 可编辑：结果必须能被作者直接改，不是只能看不能动的成品。
- 可追溯：每一场戏都能回溯到原小说章节。
- 可验证：生成结果必须能通过 Schema 校验。
- 可增量：支持单章、单场景局部重写。
- 易部署：比赛演示优先追求零复杂依赖。

## 3. 技术选型

| 层级 | 技术 | 作用 |
| --- | --- | --- |
| 前端 | React + Vite + TypeScript | 搭建交互式编辑器和工作台 |
| 后端 | Go + Gin | 提供 API、任务编排、文件导出 |
| AI | OpenAI-compatible 模型 provider + 多 Agent 编排 | 完成智能切章、结构规划、逐场生成和修复 |
| 存储 | SQLite | 保存项目、章节、脚本版本和任务状态 |
| 文件 | 本地文件系统 | 保存原文、导出 YAML、临时产物 |

## 4. 总体架构

```mermaid
flowchart LR
  A[小说文本/章节文件] --> B[React 前端工作台]
  B --> C[Go API]
  C --> D[规则切分与文本清洗]
  D --> E{章节足够}
  E -- 否 --> F[Chapter Segmentation Agent]
  E -- 是 --> G[章节列表]
  F --> G
  G --> H[StoryStructureAgent]
  H --> I[SceneExpansionAgent]
  I --> J[DraftAssembler]
  J --> K[Schema 校验与修复 Agent]
  K --> L[YAML 生成器]
  L --> M[剧本编辑器/预览/导出]
```

## 5. 前端架构

前端以“工作台”形态呈现，不做营销页。

### 核心页面

- 项目列表：进入或新建改编项目。
- 小说导入页：粘贴文本、上传文件、确认章节。
- 生成配置页：设置改编目标、风格、篇幅、节奏。
- 剧本编辑页：按场景查看、编辑、重写、导出。

### 核心组件

- `NovelUploader`：接收文本或文件。
- `ChapterSplitter`：展示章节拆分结果并允许手动修正。
- `ScriptOutline`：展示大纲、角色、场次结构。
- `YamlEditor`：编辑最终 YAML。
- `SceneInspector`：查看某一场景的来源章节、角色、对白、备注。
- `RegeneratePanel`：选择局部内容重写。

### 状态管理

- 服务器状态：用 TanStack Query 拉取项目、任务、脚本版本。
- 本地状态：用 Zustand 管理当前编辑态、选择项和面板开关。
- 编辑器状态：YAML 采用 Monaco Editor 或同类编辑器承载。

## 6. 后端架构

Go 后端负责所有 AI 调用和数据落库，前端不直接接触模型 Key。

### 模块划分

- `handler`：HTTP 接口层。
- `service`：业务编排层。
- `parser`：规则章节识别、段落清洗、字符归一。
- `ai`：OpenAI-compatible 调用、Chapter Segmentation Agent、结构规划 Agent、场景扩写 Agent、修复 Agent。
- `validator`：Schema 校验、字段补全、格式修复。
- `exporter`：YAML 序列化和文件导出。
- `repository`：SQLite 读写。

### 建议目录

```text
backend/
  cmd/api/
  internal/
    handler/
    service/
    parser/
    ai/
    validator/
    exporter/
    repository/
```

## 7. AI 生成流水线

当前默认采用多 Agent 分阶段生成，不一次性让模型吐完整剧本。

1. 规则切分：优先识别“第 X 章 / Chapter X”等章节标题。
2. 智能切章：规则失败时，Chapter Segmentation Agent 基于段落编号返回章节边界，后端重建章节正文。
3. 全局分析与结构规划：StoryStructureAgent 抽取人物、关系、时间线、主题、冲突，并产出幕结构和场景卡。
4. 场景生成：SceneExpansionAgent 按场景输出动作、对白、舞台提示。
5. 校验修复：Malformed JSON Agent 和 ValidationRepairAgent 检查字段类型、章节映射和连续性。
6. 导出落盘：输出最终 YAML，并保留版本用于回溯。

这里建议让模型先产出受约束的结构化 JSON，中间层做验证后再序列化为 YAML。这样比直接生成 YAML 更稳定，也更容易修复局部错误。

## 8. API 设计

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/projects` | 新建改编项目 |
| POST | `/api/v1/projects/:id/source` | 上传或粘贴小说原文 |
| POST | `/api/v1/projects/:id/generate` | 发起生成任务 |
| GET | `/api/v1/jobs/:id` | 查询任务状态 |
| GET | `/api/v1/projects/:id/script` | 获取当前剧本 YAML |
| PUT | `/api/v1/projects/:id/script` | 保存手动编辑结果 |
| POST | `/api/v1/projects/:id/script/regenerate` | 局部重写某场景或某段 |
| GET | `/api/v1/schema/yaml` | 提供 YAML Schema 给前端和文档页 |

## 9. 数据存储

建议最小化存储结构：

- `projects`：项目基本信息。
- `source_documents`：原小说文本和来源文件。
- `chapters`：切分后的章节内容。
- `ai_jobs`：生成任务、进度、错误日志。
- `script_versions`：每次导出的 YAML 版本。
- `edit_logs`：用户手动修改记录。

比赛演示阶段优先用 SQLite，降低部署成本；如果后续扩展多人协作，可平滑迁移到 PostgreSQL。

## 10. 可靠性与安全

- 模型 Key 只放在后端环境变量。
- 生成任务异步执行，避免前端等待超时。
- 每次生成都记录输入版本和输出版本，便于回滚。
- Schema 校验失败时只重试失败片段，不重跑全部内容。
- 导出 YAML 前做一次最终校验，避免把坏结果交给作者。

## 11. 测试策略

- 单元测试：章节识别、角色归一、YAML 序列化。
- 集成测试：模型返回结果的解析与修复。
- 接口测试：项目创建、生成、导出、保存编辑。
- 端到端测试：导入小说 -> 生成剧本 -> 手工编辑 -> 导出 YAML。

## 12. 推荐 PR 拆分

为了满足比赛的持续交付要求，建议按小粒度 PR 推进：

1. 仓库骨架与文档。
2. 小说导入与章节切分。
3. 模型生成流水线。
4. YAML 编辑与预览。
5. 局部重写、导出和演示优化。

