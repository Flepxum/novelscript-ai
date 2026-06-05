# YAML Schema 设计

## 1. 设计目标

这个 Schema 的目标不是把剧本“定死”，而是先给作者一份足够结构化、又足够好改的初稿。

- 人可以直接编辑。
- 程序可以稳定校验。
- 能追溯到原小说章节。
- 支持局部重写和版本迭代。

## 2. 设计思路

最终输出使用 YAML，但生成过程内部建议先用结构化 JSON 作为中间表示，再由后端转换成 YAML。原因很简单：

- JSON 更适合做字段校验。
- YAML 更适合作者阅读和人工修改。
- 先校验再导出，可以减少格式错误。

## 3. 根结构

```yaml
schema_version: "1.0"
project: {}
source: {}
world: {}
characters: []
acts: []
scenes: []
continuity: {}
revision: {}
```

## 4. 字段定义

### 4.1 `schema_version`

- 类型：string
- 必填：是
- 作用：用于兼容后续 Schema 升级。

### 4.2 `project`

建议字段：

- `title`：项目名。
- `adaptation_target`：改编目标，如 `web_series`、`film`、`stage_play`。
- `language`：默认 `zh-CN`。
- `genre`：题材标签。

作用：保存改编项目本身的元信息。

### 4.3 `source`

建议字段：

- `novel_title`
- `author`
- `chapter_count`
- `chapter_refs`

作用：区分“原小说”与“改编脚本”，并保留来源信息。

### 4.4 `world`

建议字段：

- `logline`
- `theme`
- `tone`
- `setting`

作用：为整部剧本提供统一的世界观、主题和基调。

### 4.5 `characters`

每个角色建议包含：

- `id`
- `name`
- `aliases`
- `role`
- `traits`
- `goal`
- `conflict`
- `arc`
- `relationships`

作用：把角色做成“实体表”，避免在每个场景里重复写长描述，也方便统一改名和改关系。

### 4.6 `acts`

每个幕建议包含：

- `id`
- `title`
- `purpose`
- `scene_ids`

作用：给编辑和评审一个清晰的整体结构，便于按幕检查节奏。

### 4.7 `scenes`

每个场景建议包含：

- `id`
- `act_id`
- `chapter_refs`
- `location`
- `time`
- `purpose`
- `conflict`
- `summary`
- `characters`
- `beats`
- `dialogues`
- `notes`

作用：场景是最适合人工修改的粒度，也是 AI 最容易稳定生成的粒度。

### 4.8 `continuity`

建议字段：

- `timeline`
- `open_threads`
- `foreshadowing`
- `props`

作用：专门处理前后连贯性，避免人物、时间线、道具和伏笔打架。

### 4.9 `revision`

建议字段：

- `generated_at`
- `confidence`
- `editor_notes`
- `ai_assumptions`

作用：记录这份稿子是怎么生成的、哪里是 AI 推断的、哪里需要人工再打磨。

## 5. 示例

```yaml
schema_version: "1.0"
project:
  title: "山河书"
  adaptation_target: "web_series"
  language: "zh-CN"
  genre: ["古装", "悬疑"]

source:
  novel_title: "山河书"
  author: "匿名"
  chapter_count: 12
  chapter_refs: [1, 2, 3]

world:
  logline: "少年卷入旧案，在追查真相时揭开王朝秘辛。"
  theme: ["真相", "责任", "身份"]
  tone: "紧张、克制、带一点宿命感"

characters:
  - id: "c01"
    name: "沈砚"
    role: "protagonist"
    traits: ["冷静", "敏锐", "隐忍"]
    goal: "查清父亲被害真相"
    conflict: "越接近真相，越发现自己也被卷入局中"

acts:
  - id: "act1"
    title: "入局"
    purpose: "建立主线冲突"
    scene_ids: ["s01"]

scenes:
  - id: "s01"
    act_id: "act1"
    chapter_refs: [1]
    location: "城南旧宅"
    time: "夜"
    purpose: "引出案件与主角动机"
    conflict: "线索被提前销毁"
    summary: "沈砚在旧宅中找到一封被烧毁一半的信。"
    characters: ["c01"]
    beats:
      - "沈砚进入旧宅"
      - "发现残信"
      - "意识到有人先一步来过"

continuity:
  timeline:
    - "旧宅案发生在主线开始前三年"
  open_threads:
    - "残信的完整内容"

revision:
  confidence: "medium"
  editor_notes:
    - "这里保留了编辑空间，后续可补完整对白。"
```

## 6. 为什么这样设计

### 6.1 `schema_version`

后续一定会迭代字段。版本号能保证旧稿子还能被正确解析，不会因为升级直接失效。

### 6.2 `project` / `source` 分离

原小说信息和改编脚本不是一类东西。分开后，既能保留来源，也不会把脚本结构搞乱。

### 6.3 `characters` 作为统一实体表

人物一旦分散写在各个场景里，后面改名、改关系、改人物弧线都会很痛。集中管理最稳。

### 6.4 `acts` + `scenes`

这符合剧本编辑的真实工作流：先看整体节奏，再改具体场景。场景级别也是 AI 最适合稳定输出的粒度。

### 6.5 `chapter_refs`

比赛要求强调“从 3 章以上小说自动转换”。保留章节映射可以证明改编关系，也方便作者回到原文核对。

### 6.6 `continuity`

连续性问题是 AI 改编最常见的坑。单独拿出来后，校验和修复就能聚焦处理，而不是让问题散落在正文里。

### 6.7 `revision`

这份稿子不是终稿，而是“可继续打磨的初稿”。记录 AI 假设和编辑备注，能让后续修改更高效。

## 7. 校验策略

- 根字段必须存在。
- `id` 必须唯一。
- `acts.scene_ids` 与 `scenes.id` 必须能互相映射。
- `characters` 引用必须指向已定义角色。
- `chapter_refs` 必须是有效章节编号。
- `schema_version` 不匹配时，进入兼容转换流程。
