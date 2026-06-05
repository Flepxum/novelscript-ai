# NovelScript AI

AI 小说转剧本工具。

项目目标是把 3 个章节以上的小说文本，自动整理成可编辑、可追溯、可导出的结构化剧本初稿，输出格式采用 YAML，方便作者继续打磨。

## 技术栈

- 前端：React + Vite + TypeScript
- 后端：Go + Gin
- AI：OpenAI Responses API
- 存储：SQLite + 本地文件
- 输出：YAML

## 文档

- [系统架构设计](docs/architecture.md)
- [YAML Schema 设计](docs/yaml-schema.md)

## 目录规划

- `frontend/`：React 编辑器、预览、导出界面
- `backend/`：Go API、文本解析、AI 编排、YAML 导出
- `docs/`：架构与 Schema 文档

## 当前状态

- 已完成整体方案设计
- 下一步进入前后端脚手架与核心流程实现

## Demo

- 视频链接：待补充
