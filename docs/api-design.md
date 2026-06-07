# API 设计

## 1. 基本约定

- Base URL：`/api/v1`
- 请求格式：JSON，文件上传使用 `multipart/form-data`
- 响应格式：JSON
- 时间格式：ISO 8601
- 鉴权：比赛演示阶段暂不启用登录，后续可扩展用户系统

## 2. 响应封装

成功响应：

```json
{
  "data": {}
}
```

失败响应：

```json
{
  "error": {
    "code": "bad_request",
    "message": "invalid request body",
    "details": []
  }
}
```

## 3. 项目接口

### 3.1 新建项目

`POST /api/v1/projects`

请求：

```json
{
  "title": "山河书改编",
  "adaptation_target": "web_series",
  "language": "zh-CN"
}
```

响应：

```json
{
  "data": {
    "id": "proj_001",
    "title": "山河书改编",
    "created_at": "2026-06-06T10:00:00+08:00"
  }
}
```

### 3.2 获取项目

`GET /api/v1/projects/:projectId`

返回项目元信息、章节数量、当前脚本版本和最近任务状态。

## 4. 小说来源接口

### 4.1 上传或粘贴小说

`POST /api/v1/projects/:projectId/source`

请求：

```json
{
  "novel_title": "山河书",
  "author": "匿名",
  "content": "第一章 ...\n第二章 ...\n第三章 ..."
}
```

响应：

```json
{
  "data": {
    "source_id": "src_001",
    "chapter_count": 3,
    "chapters": [
      {
        "index": 1,
        "title": "第一章",
        "word_count": 3200
      }
    ]
  }
}
```

后端必须拒绝少于 3 个章节的输入。

### 4.2 更新章节切分

`PUT /api/v1/projects/:projectId/chapters`

用于作者手动修正章节边界。

## 5. 生成接口

### 5.1 发起生成任务

`POST /api/v1/projects/:projectId/generate`

请求：

```json
{
  "style": "克制、悬疑、影视化",
  "target_scene_count": 12,
  "dialogue_density": "medium",
  "preserve_original_names": true
}
```

响应：

```json
{
  "data": {
    "job_id": "job_001",
    "status": "queued"
  }
}
```

### 5.2 查询任务状态

`GET /api/v1/jobs/:jobId`

响应：

```json
{
  "data": {
    "id": "job_001",
    "status": "generating_scenes",
    "progress": 62,
    "current_step": "正在生成第 8 场",
    "error": null
  }
}
```

## 6. 剧本接口

### 6.1 获取当前 YAML

`GET /api/v1/projects/:projectId/script`

响应：

```json
{
  "data": {
    "version_id": "ver_001",
    "yaml": "schema_version: \"1.0\"\nproject:\n  title: \"山河书\""
  }
}
```

### 6.2 保存编辑后的 YAML

`PUT /api/v1/projects/:projectId/script`

请求：

```json
{
  "yaml": "schema_version: \"1.0\"\nproject:\n  title: \"山河书\"",
  "editor_note": "补充第一场对白"
}
```

后端先解析 YAML，再运行 Schema 校验，成功后创建新版本。

### 6.3 局部重写

`POST /api/v1/projects/:projectId/script/regenerate`

请求：

```json
{
  "scope": "scene",
  "scene_id": "s03",
  "instruction": "加强冲突，减少旁白，增加两句对白"
}
```

响应返回修改后的场景对象和完整 YAML 版本号。

### 6.4 版本列表

`GET /api/v1/projects/:projectId/script/versions`

返回当前项目的脚本版本列表，供前端展示版本时间线。

### 6.5 获取指定版本

`GET /api/v1/projects/:projectId/script/versions/:versionId`

返回指定版本的 YAML 和结构化 `draft`，用于历史版本查看或载入编辑器。

### 6.6 恢复指定版本

`POST /api/v1/projects/:projectId/script/versions/:versionId/restore`

将指定历史版本复制为一个新的当前版本，而不是覆盖原版本。这样可以保留完整编辑历史，便于评审和作者回溯。

## 7. Schema 接口

### 7.1 获取 YAML Schema

`GET /api/v1/schema/script`

返回 `schemas/script.schema.json` 的内容。前端可以用它做编辑器提示，后端用同一份契约做校验。

