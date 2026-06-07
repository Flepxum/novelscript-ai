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
| 存储 | MemoryRepository | 比赛演示阶段保存项目、章节、任务和脚本版本 |
| 文件 | 本地配置与前端下载 | 本地 `.env` 保存配置，前端下载 YAML |

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
  G --> H[ChapterAnalysisAgent 批量章节理解]
  H --> I[StoryBibleAgent]
  I --> J[ScenePlannerAgent]
  J --> K[SceneExpansionAgent]
  K --> L[DraftAssembler]
  L --> M[Schema 校验与修复 Agent]
  M --> N[YAML 生成器]
  N --> O[剧本编辑器/预览/导出]
```

## 5. 前端架构

前端以“工作台”形态呈现，不做营销页。

### 当前页面

当前前端是一个单页三栏工作台，不做路由拆页，评委启动后可以直接完成导入、切章、生成、编辑、重写和导出。

### 核心界面区域

- 左侧：项目元信息、小说原文、章节列表和章节编辑。
- 中间：YAML 编辑器、剧本预览、分屏切换和状态提示。
- 右侧：任务进度、场景检查、局部重写、版本列表和 Schema 查看。

### 状态管理

- 使用 React `useState` / `useMemo` 管理项目、章节、任务、YAML、版本和当前选中场景。
- API 调用集中在 `frontend/src/api.ts`。
- YAML 预览使用 `yaml` 包解析，编辑器当前采用原生 textarea，降低依赖和部署复杂度。

## 6. 后端架构

Go 后端负责所有 AI 调用和数据落库，前端不直接接触模型 Key。

### 模块划分

- `handler`：HTTP 接口层。
- `service`：业务编排层。
- `parser`：规则章节识别、段落清洗、字符归一。
- `ai`：OpenAI-compatible 调用、切章 Agent、章节分析 Agent、故事圣经 Agent、场景规划 Agent、场景扩写 Agent、修复 Agent。
- `validator`：Schema 校验、字段补全、格式修复。
- `exporter`：YAML 序列化和文件导出。
- `repository`：内存仓库读写，保留替换持久化存储的边界。

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
3. 批量章节理解：ChapterAnalysisAgent 按批次为每章提炼事件、人物、地点、冲突和改编提示，降低长篇小说前置调用次数。
4. 故事圣经：StoryBibleAgent 统一世界观、角色 ID、关系、时间线和伏笔。
5. 场景规划：ScenePlannerAgent 生成幕结构和无对白场景卡。
6. 场景生成：SceneExpansionAgent 按场景输出动作、对白、舞台提示。
7. 校验修复：Malformed JSON Agent 和 ValidationRepairAgent 检查字段类型、章节映射和连续性。
8. 导出落盘：输出最终 YAML，并保留版本用于回溯。

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

比赛演示阶段使用 `MemoryRepository`，降低部署和数据库初始化成本；如果后续扩展多人协作，可在 `repository` 边界替换为 SQLite 或 PostgreSQL。

## 10. 可靠性与安全

- 模型 Key 只放在后端环境变量。
- 生成任务异步执行，避免前端等待超时。
- 每次生成都记录输入版本和输出版本，便于回滚。
- 长篇章节分析按批次并发执行，模型漏章时使用原文摘要兜底并写入后端日志。
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

