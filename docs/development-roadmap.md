# 开发路线与 PR 规划

## 1. 原则

比赛规则强调持续交付和 PR 质量，因此项目不要最后一天一次性导入。每个 PR 只做一件事，并保证合并后主分支可运行。

## 2. 推荐里程碑

| 阶段 | 目标 | 可演示结果 |
| --- | --- | --- |
| M1 | 文档和工程骨架 | README、架构文档、Schema 文档 |
| M2 | 小说导入和章节切分 | 粘贴文本后看到章节列表 |
| M3 | 后端生成任务 | 生成编排层能产出 YAML |
| M4 | 模型 provider | 3 章小说可生成模型辅助剧本初稿 |
| M5 | 前端编辑器 | 可编辑、保存、导出 YAML |
| M6 | 局部重写和版本 | 单场景重写、版本回滚 |
| M7 | Demo 打磨 | 示例数据、视频脚本、README 完整 |

## 3. PR 拆分建议

### PR 1：初始化文档与 Schema

- 功能描述：补充系统架构、YAML Schema 和开发路线。
- 实现思路：先定义契约，再按契约实现前后端。
- 测试方式：检查文档链接和 JSON Schema 是否可解析。

### PR 2：初始化 Go 后端骨架

- 功能描述：添加 Gin 服务、配置读取、健康检查。
- 实现思路：建立 `cmd/api` 和 `internal` 分层目录。
- 测试方式：`go test ./...`，访问 `/api/v1/health`。

### PR 3：小说章节切分

- 功能描述：支持粘贴小说并自动识别 3 个以上章节。
- 实现思路：用正则识别中文章节和英文 Chapter 标题，保留手动修正接口。
- 测试方式：用 `backend/testdata` 覆盖中文、英文、无标题文本。

### PR 4：剧本 Schema 校验与 YAML 导出

- 功能描述：将结构化剧本对象导出为 YAML，并校验引用关系。
- 实现思路：YAML 解析后先跑 JSON Schema，再跑业务校验。
- 测试方式：覆盖缺字段、非法角色引用、非法章节引用。

### PR 5：剧本生成编排层

- 功能描述：接入 OpenAI-compatible 模型服务，生成真实结构化初稿。
- 实现思路：后端读取 `MODEL_BASE_URL`、`MODEL_API_KEY`、`MODEL_NAME`，调用 `chat/completions` 并解析结构化 JSON。
- 测试方式：使用本地 HTTP 测试服务验证请求格式和结构化响应解析。

### PR 6：模型 provider 质量增强

- 功能描述：增强模型输出修复、失败重试和长文本分段能力。
- 实现思路：分阶段 prompt，结构化 JSON 输出，失败片段局部重试。
- 测试方式：用短篇 3 章样例和真实模型配置跑通一次生成。

### PR 7：React 前端骨架

- 功能描述：搭建工作台路由、基础布局和 API client。
- 实现思路：Vite + React + TypeScript，三栏编辑工作台。
- 测试方式：`npm run build`，页面可进入项目列表。

### PR 8：导入与生成交互

- 功能描述：前端支持导入小说、确认章节、发起生成。
- 实现思路：TanStack Query 管理请求和任务轮询。
- 测试方式：完整跑通导入到生成。

### PR 9：YAML 编辑器与导出

- 功能描述：支持编辑、校验、保存、下载 YAML。
- 实现思路：Monaco Editor + Schema 错误路径提示。
- 测试方式：编辑后保存为新版本并下载文件。

### PR 10：Demo 完整化

- 功能描述：补示例小说、演示脚本、README 视频链接。
- 实现思路：稳定 demo 数据优先，保证评委能复现。
- 测试方式：按 README 从零启动并完成一次演示。

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

- `docs: add yaml schema design`
- `feat(api): add project creation endpoint`
- `feat(parser): split novel chapters`
- `feat(ai): add script generation pipeline`
- `feat(frontend): add script editor layout`
- `test(parser): cover chinese chapter headings`

Commit 保持小而清楚，方便评委看到持续推进过程。

