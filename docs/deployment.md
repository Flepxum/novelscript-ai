# Docker 部署说明

本文档说明如何把 NovelScript AI 部署到服务器。部署形态为前端 Nginx 容器 + 后端 Go API 容器，Nginx 负责静态资源和 `/api/` 反向代理。

## 部署结构

```text
用户浏览器
  |
  | http://服务器IP:80
  v
frontend 容器：Nginx
  |- 托管 React/Vite 构建产物
  |- /api/ 反向代理到 backend:8080
  v
backend 容器：Go API
  |- OpenAI-compatible LLM 调用
  |- 多 Agent 小说转剧本流水线
  |- YAML Schema 校验
  |- /app/data 持久化数据卷
```

## 服务器准备

服务器需要安装 Docker Engine 和 Docker Compose 插件。

检查命令：

```bash
docker --version
docker compose version
```

## 配置后端环境变量

实际配置文件不提交到仓库。请在服务器项目目录创建：

```bash
backend/.env
```

最小可运行配置：

```text
MODEL_BASE_URL=<your-openai-compatible-base-url>
MODEL_API_KEY=<your-api-key>
MODEL_NAME=<your-model-name>
PUBLIC_BASE_URL=http://114.67.96.157
CORS_ALLOWED_ORIGINS=http://114.67.96.157
```

常用生产配置：

```text
MODEL_TIMEOUT_SECONDS=180
MODEL_MAX_RETRIES=2
MODEL_MAX_INPUT_CHARS=24000
MODEL_MAX_OUTPUT_TOKENS=6000
MODEL_TEMPERATURE=1
MODEL_AGENT_PIPELINE=multi_agent
MODEL_CHAPTER_ANALYSIS_BATCH_SIZE=6
MODEL_SCENE_EXPANSION_BATCH_SIZE=3
MODEL_FAST_PATH_MAX_CHARS=5000
JOB_TIMEOUT_SECONDS=300
JOB_MAX_PARALLEL=2
REQUEST_BODY_LIMIT_MB=20
```

## 构建并启动

在仓库根目录执行：

```bash
docker compose up -d --build
```

查看容器状态：

```bash
docker compose ps
```

查看后端日志：

```bash
docker compose logs -f backend
```

查看前端 Nginx 日志：

```bash
docker compose logs -f frontend
```

## 访问和健康检查

前端访问：

```text
http://114.67.96.157
```

后端健康检查会经过 Nginx 代理：

```bash
curl http://114.67.96.157/api/v1/health
```

前端容器健康检查：

```bash
curl http://114.67.96.157/healthz
```

## 更新部署

拉取最新代码后重新构建：

```bash
git pull origin main
docker compose up -d --build
```

清理未使用镜像：

```bash
docker image prune -f
```

## 停止服务

停止但保留数据卷：

```bash
docker compose down
```

停止并删除数据卷：

```bash
docker compose down -v
```

删除数据卷会清空内存仓库之外的挂载数据，请谨慎操作。

## 注意事项

- `backend/.env` 不要提交到仓库。
- 前端生产环境使用相对路径请求 `/api/v1`，不需要写死 API 地址。
- 后端容器内 Schema 路径固定为 `/app/schemas/script.schema.json`。
- 当前仓库使用内存仓库作为演示存储，容器重启后内存中的项目状态会丢失；后续如切换 SQLite，可继续使用 `/app/data` 数据卷持久化。
