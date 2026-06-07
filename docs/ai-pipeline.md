# AI 生成流水线设计

## 1. 目标

AI 不是直接替作者写终稿，而是把小说原文转换成一份结构清晰、能被继续编辑的剧本初稿。流水线必须兼顾稳定性、可追溯性和可修复性。

## 2. 输入要求

- 至少 3 个章节。
- 支持粘贴文本和上传 `.txt` 文件。
- 后端统一做编码、换行、空白字符清洗。
- 每个章节保留 `chapter_index`、`title`、`content`、`word_count`。

## 3. 分阶段流程

```mermaid
flowchart TD
  A[小说原文] --> B[文本清洗]
  B --> C{规则章节切分成功}
  C -- 是 --> D[章节列表]
  C -- 否 --> E[Chapter Segmentation Agent]
  E --> F[段落边界 JSON]
  F --> D
  D --> G[ChapterAnalysisAgent 批量章节理解]
  G --> H[章节分析 JSON]
  H --> I[StoryBibleAgent]
  I --> J[世界观 人物 连续性]
  J --> K[ScenePlannerAgent]
  K --> L[幕结构与场景卡]
  L --> M[SceneExpansionAgent]
  M --> N[逐场景节拍 动作 对白]
  N --> O[DraftAssembler]
  O --> P[ScriptDraft JSON]
  P --> Q{JSON 解析成功}
  Q -- 否 --> R[MalformedJSONRepairAgent]
  R --> P
  Q -- 是 --> S[Schema 与业务校验]
  S --> T{校验通过}
  T -- 是 --> U[转换 YAML]
  T -- 否 --> V[ValidationRepairAgent]
  V --> S
```

## 4. 为什么不一次生成完整 YAML

长篇小说改编很容易遇到上下文过长、结构漂移、角色名字不一致、YAML 缩进错误等问题。分阶段生成可以把错误控制在更小范围：

- 章节摘要错了，只重跑摘要。
- 人物表错了，只重跑人物抽取。
- 某一场景不合格，只重跑该场景。

当前生产默认使用 `MODEL_AGENT_PIPELINE=multi_agent`。直接整稿生成只作为调试或兼容路径；真实生成会先批量分析章节，再建立故事圣经、规划场景、逐场扩写，最后由后端组装为 `ScriptDraft` 并导出 YAML。

## 4.1 长文本章节切分策略

不规范小说可能没有“第 X 章”标题，也可能一次粘贴十几万字。Chapter Segmentation Agent 不要求模型返回小说正文，而是把清洗后的文本切成带编号的段落窗口，让模型只返回：

```json
{
  "chapters": [
    {
      "title": "雨夜书店",
      "start_paragraph": 1,
      "confidence": "high",
      "reason": "开篇建立主要地点和人物"
    }
  ]
}
```

后端根据 `start_paragraph` 从原文重建章节正文。这样做有三个原因：

- 保留原文：模型不复述正文，避免改写、漏字或幻觉污染来源内容。
- 适配长文：段落索引可以分窗口处理，模型只看当前窗口的摘要级段落预览。
- 可追溯：后续剧本 `chapter_refs` 仍能回到原始章节边界。

## 4.2 Agent 职责拆分

| Agent | 输入 | 输出 | 可靠性策略 |
| --- | --- | --- | --- |
| ChapterSegmentationAgent | 清洗后的原文段落窗口 | 章节标题与 `start_paragraph` | 不返回正文；后端用原文重建章节；边界不足时进入修复 |
| ChapterAnalysisAgent | 一批章节原文 | `chapterBriefBatch`：每章摘要、事件、人物、地点、冲突、改编提示 | `MODEL_CHAPTER_ANALYSIS_BATCH_SIZE` 控制每批章节数；批次可按 `JOB_MAX_PARALLEL` 并发；模型漏章时后端用原文摘要兜底并记录日志 |
| StoryBibleAgent | 全部 `chapterBrief` | `world`、`characters`、`continuity` | 统一角色 ID、关系、时间线，避免后续场景角色漂移 |
| ScenePlannerAgent | 故事圣经与章节分析 | `acts` 与无对白 `scene cards` | 只规划场景目标、冲突、来源章节和角色，不生成对白 |
| SceneExpansionAgent | 单个场景卡、故事计划、相关章节原文 | 完整 `Scene`，含节拍、动作、对白 | 可并发逐场生成；失败时只修复或重跑单场 |
| DraftAssembler | 结构计划和完整场景 | `ScriptDraft` | 程序组装元数据、章节引用、版本时间，减少模型自由度 |
| MalformedJSONRepairAgent | 解析失败的原始输出和上下文 | 修复后的 JSON | 专门补齐截断、去掉 Markdown、修正闭合结构 |
| ValidationRepairAgent | 后端校验问题和当前 `ScriptDraft` | 修复后的 `ScriptDraft` | 只修字段、引用、ID 和缺失结构，不重写有效剧情 |
| SceneRegenerationAgent | 当前剧本、指定场景、用户指令 | 更新后的完整剧本与目标场景 | 支持作者对单场进行局部打磨 |

## 5. 中间数据契约

AI 中间输出采用 JSON：

- `chapter_boundaries`
- `chapter_briefs`
- `story_bible`
- `scene_plan`
- `scene_drafts`
- `script_draft`

后端把这些结构合并成 `ScriptDraft`，校验通过后再导出 YAML。模型响应如果出现截断 JSON、Markdown 包裹、字段缺失或引用错误，会进入对应 agent 修复步骤，而不是直接把坏结果交给前端。

## 6. Prompt 结构

每个阶段的提示词都包含四部分：

- 任务目标：说明当前阶段只做什么。
- 输入边界：给出章节、摘要或上一阶段结果。
- 输出结构：要求返回 JSON 对象。
- 质量约束：要求保留章节引用、避免新增无法追溯的关键情节。

示例骨架：

```text
你是小说改编剧本助理。
请根据输入章节生成场景计划。
只输出 JSON，不要输出解释。
每个场景必须包含 chapter_refs、purpose、conflict、characters。
不要凭空新增核心人物；如果必须推断，请写入 ai_assumptions。
```

## 7. 模型接入

当前实现通过后端真实调用 OpenAI-compatible 模型 provider。设计约束如下：

- 模型配置只能放在后端环境变量或配置文件中，前端不接触 Key。
- 模型名称不写死在前端。
- 对关键阶段使用结构化输出约束，降低解析失败概率。
- 所有模型调用通过 `internal/ai` 包封装，便于替换 provider。
- 默认启用 `MODEL_AGENT_PIPELINE=multi_agent`，按章节切分、批量章节分析、故事圣经、场景规划、场景扩写、修复校验分阶段调用模型。
- `MODEL_CHAPTER_ANALYSIS_BATCH_SIZE` 控制 ChapterAnalysisAgent 每次请求分析多少章，默认 6。长篇小说的章节分析请求数约为 `ceil(chapter_count / batch_size)`。
- `JOB_MAX_PARALLEL` 控制可并发的章节分析批次和场景扩写数量，兼顾速度和 provider 限流。

必需配置项：

```text
MODEL_PROVIDER=
MODEL_BASE_URL=
MODEL_API_KEY=
MODEL_NAME=
MODEL_TIMEOUT_SECONDS=120
MODEL_MAX_RETRIES=2
MODEL_AGENT_PIPELINE=multi_agent
MODEL_CHAPTER_ANALYSIS_BATCH_SIZE=6
```

### 7.1 长篇性能模型

多 Agent 流水线的耗时主要由 LLM 请求数量和 provider 并发限制决定。当前生成请求数近似为：

```text
ceil(chapter_count / MODEL_CHAPTER_ANALYSIS_BATCH_SIZE)
+ 1 次 StoryBibleAgent
+ 1 次 ScenePlannerAgent
+ scene_count 次 SceneExpansionAgent
+ 必要时的 JSON/校验修复请求
```

其中章节理解阶段按批次并发，逐场扩写阶段按场景并发。这样既避免一次性生成完整剧本导致 `finish_reason=length` 或 JSON 截断，也避免长篇小说在前置章节分析阶段被一章一请求拖慢。

## 8. 校验与修复

校验分两层：

- Schema 校验：字段、类型、必填项。
- 业务校验：章节引用、角色引用、场景归属、ID 唯一性。

修复策略：

1. 能由程序补齐的字段直接补齐，例如空数组。
2. JSON 解析失败时进入 Malformed JSON 修复 Agent，结合原始输出和输入上下文补齐完整 `ScriptDraft`。
3. 引用错误、结构缺失交给结构校验修复 Agent。
4. 修复超过次数后标记任务失败，并把错误展示给前端，同时后端日志保留 LLM 步骤、状态码和响应摘要。

## 9. 真实生成流程

比赛 demo 使用真实模型生成。为了让结果可校验，后端采用以下策略：

- 使用 `chat/completions` 请求结构化 JSON。
- 使用 `response_format` 约束输出格式，默认优先使用 JSON Schema。
- `internal/ai` 中的 `ChapterSegmentationAgent` 负责不规范长文本切章。
- `internal/ai` 中的 `ScriptAgent` 负责 ChapterAnalysisAgent、StoryBibleAgent、ScenePlannerAgent、SceneExpansionAgent、DraftAssembler、局部重写和修复编排。
- 默认生产流程先批量分析章节，再建立故事圣经，再规划场景卡，再逐场调用 scene agent 生成对白和动作，最后组装完整 `ScriptDraft`。
- 如果调试模式下使用完整剧本生成并触发 `finish_reason=length`，Agent 会自动切换到分解式流程。
- 模型输出先解析成 `ScriptDraft`，通过业务校验后再导出 YAML。
- `backend/testdata/` 仅用于提供可重复导入的小说样例，不替代模型调用。

如果模型配置缺失、网络失败或输出不符合引用规则，任务会进入 failed 状态并把错误展示给前端。

## 10. 可解释性设计

输出的每个场景都保留：

- `chapter_refs`
- `purpose`
- `conflict`
- `ai_assumptions`

作者可以看到 AI 为什么这样改，而不是只拿到一段不可解释的文本。

