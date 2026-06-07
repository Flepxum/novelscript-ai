# 开发路线与 PR 规划

## 1. 当前完成度

项目已经完成从小说输入到结构化剧本 YAML 的主链路：

- 前端单页工作台：导入、切分、章节编辑、智能场数、生成、YAML 编辑、预览、版本、局部重写、复制和导出。
- 后端 API：项目、来源、章节、异步生成任务、脚本保存、版本、恢复、局部重写、Schema 读取。
- AI 能力：OpenAI-compatible 真实模型接入，多 Agent 章节切分、批量章节分析、故事圣经、场景规划、逐场扩写、JSON 修复和结构修复。
- 校验导出：YAML 解析、业务引用校验、版本化保存。
- 文档：README、架构、AI 流水线、API、前端、后端、YAML Schema、Demo 脚本。

## 2. 已形成的能力拆分

| 能力 | 状态 | 说明 |
| --- | --- | --- |
| 工程骨架 | 已完成 | `frontend/`、`backend/`、`docs/`、`schemas/` 同仓管理 |
| 小说导入 | 已完成 | 粘贴文本和上传 `.txt` |
| 章节切分 | 已完成 | 规则切分 + ChapterSegmentationAgent fallback |
| 章节修订 | 已完成 | 前端可编辑标题和正文 |
| 真实 LLM 接入 | 已完成 | 后端读取 `MODEL_BASE_URL`、`MODEL_API_KEY`、`MODEL_NAME` |
| 多 Agent 生成 | 已完成 | ChapterAnalysisAgent、StoryBibleAgent、ScenePlannerAgent、SceneExpansionAgent |
| 自适应 Agent 编排 | 已完成 | 短篇走 StoryStructureAgent 快速路径，长篇走批量章节分析 |
| 长篇加速编排 | 已完成 | ChapterAnalysisAgent 支持批量章节分析，SceneExpansionAgent 支持批量场景扩写 |
| 智能场数 | 已完成 | `target_scene_count=0` 时由 Agent 决定 |
| YAML 编辑保存 | 已完成 | 后端解析和校验后创建新版本 |
| 局部重写 | 已完成 | 单场景重写并生成新版本 |
| 版本恢复 | 已完成 | 历史版本复制为当前新版本 |
| Schema 文档 | 已完成 | `docs/yaml-schema.md` 和 `schemas/script.schema.json` |

## 3. 后续可拆小 PR

后续如果继续迭代，仍应保持“一 PR 一件事”：

1. 持久化存储 PR：用 SQLite 替换 `MemoryRepository`，保留现有 repository 接口。
2. 任务事件 PR：把轮询升级为 Server-Sent Events，实时展示 Agent 步骤。
3. Agent 追踪 PR：为每个 LLM 调用记录 request id、agent name、耗时、token 估算和错误摘要。
4. YAML 编辑增强 PR：增加行号、Schema path 高亮和保存前差异预览。
5. Demo 视频 PR：补 README 视频链接和实际演示截图。

## 4. PR 描述模板

```markdown
## 功能描述

说明本 PR 新增或修改了什么，用户怎么使用。

## 实现思路

说明核心技术选型、关键模块和主要逻辑。

## 测试方式

- [ ] 本地命令
- [ ] 手动验证步骤
- [ ] 截图或录屏说明

## 备注

如复用历史代码、第三方库、示例数据来源，需要在这里说明。
```

## 5. Commit 建议

- `docs: refresh project architecture`
- `feat(ai): add chapter segmentation agent`
- `feat(ai): orchestrate script agents`
- `feat(frontend): add smart scene count`
- `test(ai): cover multi-agent pipeline`

Commit 保持小而清楚，方便评委看到持续推进过程。
