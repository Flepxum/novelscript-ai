# 前端架构设计

## 1. 前端定位

前端是面向小说作者的单页改编工作台，不做营销页。作者可以在同一个界面完成小说导入、章节确认、Agent 生成、YAML 编辑、剧本预览、局部重写、版本恢复和导出。

## 2. 技术栈

- React：构建工作台界面和交互状态。
- Vite：提供开发服务、代理和生产构建。
- TypeScript：约束 API 数据结构和剧本结构。
- lucide-react：提供按钮图标。
- yaml：把 YAML 文本解析为前端预览对象。

当前实现没有引入路由、全局状态库或 Monaco Editor，目的是降低比赛演示部署成本，让评委拉取后能直接运行。

## 3. 工作台布局

页面采用三栏布局：

- 左侧：项目名、小说名、作者、风格、智能场数、原文输入、章节列表和章节编辑。
- 中间：YAML 编辑器、剧本预览和分屏模式。
- 右侧：任务进度、场景详情、局部重写、版本列表和 Schema 查看。

在窄屏下，工作台会折叠为纵向布局，保证基本查看和轻量编辑可用。

## 4. 核心交互

| 功能 | 前端行为 | 后端接口 |
| --- | --- | --- |
| 创建项目 | 首次导入或生成前自动创建项目 | `POST /api/v1/projects` |
| 导入小说 | 粘贴或上传 `.txt`，发送全文 | `POST /api/v1/projects/:id/source` |
| 章节修订 | 编辑标题和正文后保存 | `PUT /api/v1/projects/:id/chapters` |
| 智能场数 | 默认传 `target_scene_count=0`，由 Agent 决定场数 | `POST /api/v1/projects/:id/generate` |
| 任务轮询 | 每 420ms 查询一次任务状态 | `GET /api/v1/jobs/:jobId` |
| YAML 编辑 | textarea 编辑 YAML，保存时后端解析和校验 | `PUT /api/v1/projects/:id/script` |
| 剧本预览 | 前端用 `yaml` 解析 YAML 后渲染幕、场景和对白 | `GET /api/v1/projects/:id/script` |
| 局部重写 | 对所选场景发送重写指令 | `POST /api/v1/projects/:id/script/regenerate` |
| 版本管理 | 载入或恢复历史版本 | 版本列表、详情和恢复接口 |

## 5. 状态设计

当前状态集中在 `frontend/src/App.tsx`：

- 项目状态：`projectId`、`projectTitle`、`novelTitle`、`author`。
- 来源状态：`novelText`、`chapters`、`selectedChapterIndex`、`chaptersDirty`。
- 生成配置：`style`、`sceneCount`、`smartSceneCount`。
- 任务状态：`job`、`busy`、`message`、错误弹窗 `notice`。
- 剧本状态：`yamlText`、`draft`、`selectedSceneId`、`versions`。
- 视图状态：`mode`，支持 `yaml`、`preview`、`split`。

API 请求集中在 `frontend/src/api.ts`，类型定义集中在 `frontend/src/types.ts`。

## 6. 错误提示

前端使用弹窗展示明显错误，避免 LLM 或后端失败只出现在状态栏中。错误弹窗包含：

- 标题：例如“生成失败”“导入失败”。
- 说明：面向作者的简短提示。
- 详情：后端返回的错误 message 或 validation details。

这能帮助排查模型配置缺失、LLM 输出解析失败、章节数量不足、YAML 校验失败等问题。

## 7. 演示友好设计

- 内置 `sampleNovel`，不依赖现场复制长文本。
- 支持上传 `.txt`，但上传后仍以 JSON 方式提交给后端。
- 默认启用“智能场数”，展示 Agent 可以根据章节密度决定场景数量。
- 任务面板展示 Agent 流程进度，方便 demo 讲解真实生成链路。
- 版本列表可展示生成、保存、恢复和局部重写后的历史记录。

## 8. 测试重点

- `npm run build` 必须通过。
- 导入示例小说后章节列表可编辑并保存。
- 智能场数开启时生成请求发送 `target_scene_count=0`。
- 生成成功后 YAML 和预览同步。
- 局部重写后版本列表新增版本。
- 后端失败时弹窗能展示明确错误。
