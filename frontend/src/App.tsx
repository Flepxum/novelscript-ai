import {
  CheckCircle2,
  Clipboard,
  Download,
  FilePlus2,
  FileText,
  Loader2,
  Play,
  RefreshCcw,
  Save,
  Sparkles,
  Upload
} from "lucide-react";
import { useMemo, useState } from "react";
import { parse } from "yaml";
import { api } from "./api";
import { sampleNovel } from "./data/sampleNovel";
import type { Chapter, Job, Scene, ScriptDraft, VersionItem } from "./types";

type ViewMode = "yaml" | "preview" | "split";

const delay = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms));

function App() {
  const [projectId, setProjectId] = useState("");
  const [projectTitle, setProjectTitle] = useState("雨夜书店改编");
  const [novelTitle, setNovelTitle] = useState("雨夜书店");
  const [author, setAuthor] = useState("示例作者");
  const [novelText, setNovelText] = useState(sampleNovel);
  const [chapters, setChapters] = useState<Chapter[]>([]);
  const [selectedChapterIndex, setSelectedChapterIndex] = useState(0);
  const [chaptersDirty, setChaptersDirty] = useState(false);
  const [style, setStyle] = useState("克制、悬疑、影视化");
  const [sceneCount, setSceneCount] = useState(8);
  const [job, setJob] = useState<Job | null>(null);
  const [yamlText, setYamlText] = useState("");
  const [draft, setDraft] = useState<ScriptDraft | null>(null);
  const [versions, setVersions] = useState<VersionItem[]>([]);
  const [selectedSceneId, setSelectedSceneId] = useState("s01");
  const [mode, setMode] = useState<ViewMode>("split");
  const [regenerateInstruction, setRegenerateInstruction] = useState("加强冲突，减少旁白，增加两句对白");
  const [schemaText, setSchemaText] = useState("");
  const [message, setMessage] = useState("后端连接后即可开始");
  const [busy, setBusy] = useState(false);

  const parsedDraft = useMemo(() => {
    if (draft) {
      return draft;
    }
    if (!yamlText.trim()) {
      return null;
    }
    try {
      return parse(yamlText) as ScriptDraft;
    } catch {
      return null;
    }
  }, [draft, yamlText]);

  const selectedScene = useMemo(() => {
    return parsedDraft?.scenes?.find((scene) => scene.id === selectedSceneId) ?? parsedDraft?.scenes?.[0] ?? null;
  }, [parsedDraft, selectedSceneId]);

  const selectedChapter = chapters[selectedChapterIndex] ?? null;

  const characterName = (id: string) => parsedDraft?.characters?.find((item) => item.id === id)?.name ?? id;

  async function ensureProject() {
    if (projectId) {
      return projectId;
    }
    const project = await api.createProject({
      title: projectTitle,
      adaptation_target: "web_series",
      language: "zh-CN"
    });
    setProjectId(project.id);
    return project.id;
  }

  async function importSource() {
    setBusy(true);
    setMessage("正在导入小说并切分章节");
    try {
      const id = await ensureProject();
      const result = await api.saveSource(id, {
        novel_title: novelTitle,
        author,
        content: novelText
      });
      setChapters(result.chapters);
      setSelectedChapterIndex(0);
      setChaptersDirty(false);
      setMessage(`已识别 ${result.chapter_count} 个章节`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "导入失败");
    } finally {
      setBusy(false);
    }
  }

  function updateSelectedChapter(patch: Partial<Chapter>) {
    setChapters((current) =>
      current.map((chapter, index) => (index === selectedChapterIndex ? { ...chapter, ...patch } : chapter))
    );
    setChaptersDirty(true);
  }

  async function saveChapterReview(id = projectId) {
    if (!id) {
      setMessage("请先创建项目");
      return;
    }
    if (chapters.length < 3) {
      setMessage("章节数量至少为 3");
      return;
    }
    setBusy(true);
    try {
      const result = await api.updateChapters(id, chapters);
      setChapters(result.chapters);
      setNovelText(joinChapterText(result.chapters));
      setSelectedChapterIndex((index) => Math.min(index, result.chapters.length - 1));
      setChaptersDirty(false);
      setMessage(`章节已保存：${result.chapter_count} 章`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "章节保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function generateScript() {
    setBusy(true);
    setMessage("正在生成剧本初稿");
    try {
      const id = await ensureProject();
      if (chapters.length === 0) {
        const result = await api.saveSource(id, {
          novel_title: novelTitle,
          author,
          content: novelText
        });
        setChapters(result.chapters);
        setSelectedChapterIndex(0);
        setChaptersDirty(false);
      } else if (chaptersDirty) {
        const result = await api.updateChapters(id, chapters);
        setChapters(result.chapters);
        setNovelText(joinChapterText(result.chapters));
        setChaptersDirty(false);
      }
      const started = await api.generate(id, {
        style,
        target_scene_count: sceneCount,
        dialogue_density: "medium",
        preserve_original_names: true
      });
      let next = await api.getJob(started.job_id);
      setJob(next);
      while (!["succeeded", "failed"].includes(next.status)) {
        await delay(420);
        next = await api.getJob(started.job_id);
        setJob(next);
      }
      if (next.status === "failed") {
        throw new Error(next.error ?? "生成失败");
      }
      const script = await api.getScript(id);
      setYamlText(script.yaml);
      setDraft(script.draft);
      setSelectedSceneId(script.draft.scenes[0]?.id ?? "s01");
      setMode("split");
      await refreshVersions(id);
      setMessage("剧本初稿已生成");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "生成失败");
    } finally {
      setBusy(false);
    }
  }

  async function saveYaml() {
    if (!projectId) {
      setMessage("请先生成项目");
      return;
    }
    setBusy(true);
    try {
      const script = await api.saveScript(projectId, yamlText, "前端编辑保存");
      setYamlText(script.yaml);
      setDraft(script.draft);
      await refreshVersions(projectId);
      setMessage("YAML 已保存并通过校验");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function regenerateScene() {
    if (!projectId || !selectedScene) {
      setMessage("请先选择场景");
      return;
    }
    setBusy(true);
    try {
      const script = await api.regenerateScene(projectId, selectedScene.id, regenerateInstruction);
      setYamlText(script.yaml);
      setDraft(script.draft);
      await refreshVersions(projectId);
      setMessage(`${selectedScene.id} 已局部重写`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "重写失败");
    } finally {
      setBusy(false);
    }
  }

  async function refreshVersions(id = projectId) {
    if (!id) {
      return;
    }
    const items = await api.listVersions(id);
    setVersions(items);
  }

  async function loadVersion(versionId: string) {
    if (!projectId) {
      return;
    }
    setBusy(true);
    try {
      const script = await api.getScriptVersion(projectId, versionId);
      setYamlText(script.yaml);
      setDraft(script.draft);
      setSelectedSceneId(script.draft.scenes[0]?.id ?? "s01");
      setMessage(`已载入 ${versionId}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "版本载入失败");
    } finally {
      setBusy(false);
    }
  }

  async function restoreVersion(versionId: string) {
    if (!projectId) {
      return;
    }
    setBusy(true);
    try {
      const script = await api.restoreScriptVersion(projectId, versionId);
      setYamlText(script.yaml);
      setDraft(script.draft);
      setSelectedSceneId(script.draft.scenes[0]?.id ?? "s01");
      await refreshVersions(projectId);
      setMessage(`已恢复 ${versionId}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "版本恢复失败");
    } finally {
      setBusy(false);
    }
  }

  async function loadSchema() {
    try {
      const schema = await api.schema();
      setSchemaText(JSON.stringify(schema, null, 2));
      setMessage("Schema 已加载");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Schema 加载失败");
    }
  }

  async function copyYaml() {
    await navigator.clipboard.writeText(yamlText);
    setMessage("YAML 已复制");
  }

  function downloadYaml() {
    const blob = new Blob([yamlText], { type: "text/yaml;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${projectTitle || "novelscript"}.yaml`;
    a.click();
    URL.revokeObjectURL(url);
    setMessage("YAML 已下载");
  }

  async function handleFile(file?: File) {
    if (!file) {
      return;
    }
    setNovelText(await file.text());
    setNovelTitle(file.name.replace(/\.txt$/i, ""));
    setMessage("文本文件已载入");
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">
          <Sparkles size={22} />
          <div>
            <strong>NovelScript AI</strong>
            <span>小说转结构化剧本工作台</span>
          </div>
        </div>
        <div className="toolbar">
          <button onClick={() => setNovelText(sampleNovel)} title="载入示例小说">
            <FilePlus2 size={16} />
            示例
          </button>
          <label className="button" title="上传 txt 文件">
            <Upload size={16} />
            上传
            <input type="file" accept=".txt,text/plain" onChange={(event) => handleFile(event.target.files?.[0])} />
          </label>
          <button onClick={importSource} disabled={busy}>
            <FileText size={16} />
            切分
          </button>
          <button className="primary" onClick={generateScript} disabled={busy}>
            {busy ? <Loader2 className="spin" size={16} /> : <Play size={16} />}
            生成
          </button>
          <button onClick={saveYaml} disabled={!yamlText || busy}>
            <Save size={16} />
            保存
          </button>
          <button onClick={copyYaml} disabled={!yamlText}>
            <Clipboard size={16} />
            复制
          </button>
          <button onClick={downloadYaml} disabled={!yamlText}>
            <Download size={16} />
            导出
          </button>
        </div>
      </header>

      <main className="workspace">
        <aside className="pane source-pane">
          <section className="section">
            <div className="section-title">项目</div>
            <label>
              <span>项目名</span>
              <input value={projectTitle} onChange={(event) => setProjectTitle(event.target.value)} />
            </label>
            <div className="field-row">
              <label>
                <span>小说名</span>
                <input value={novelTitle} onChange={(event) => setNovelTitle(event.target.value)} />
              </label>
              <label>
                <span>作者</span>
                <input value={author} onChange={(event) => setAuthor(event.target.value)} />
              </label>
            </div>
            <div className="field-row">
              <label>
                <span>风格</span>
                <input value={style} onChange={(event) => setStyle(event.target.value)} />
              </label>
              <label className="numeric">
                <span>场数</span>
                <input
                  type="number"
                  min={3}
                  max={18}
                  value={sceneCount}
                  onChange={(event) => setSceneCount(Number(event.target.value))}
                />
              </label>
            </div>
          </section>

          <section className="section grow">
            <div className="section-title">原文</div>
            <textarea
              className="novel-input"
              value={novelText}
              onChange={(event) => setNovelText(event.target.value)}
              spellCheck={false}
            />
          </section>

          <section className="section chapter-list">
            <div className="section-title">章节</div>
            {chapters.length === 0 ? (
              <p className="muted">尚未切分</p>
            ) : (
              <>
                <div className="chapter-picker">
                  {chapters.map((chapter, index) => (
                    <button
                      className={`chapter-item ${selectedChapterIndex === index ? "selected" : ""}`}
                      key={chapter.index}
                      onClick={() => setSelectedChapterIndex(index)}
                    >
                      <strong>{chapter.title}</strong>
                      <span>{chapter.word_count} 字</span>
                    </button>
                  ))}
                </div>
                {selectedChapter ? (
                  <div className="chapter-editor">
                    <label>
                      <span>标题</span>
                      <input
                        value={selectedChapter.title}
                        onChange={(event) => updateSelectedChapter({ title: event.target.value })}
                      />
                    </label>
                    <label>
                      <span>正文</span>
                      <textarea
                        value={selectedChapter.content ?? ""}
                        onChange={(event) => updateSelectedChapter({ content: event.target.value })}
                        spellCheck={false}
                      />
                    </label>
                    <button className="wide" onClick={() => saveChapterReview()} disabled={busy || !chaptersDirty}>
                      <Save size={16} />
                      保存章节
                    </button>
                  </div>
                ) : null}
              </>
            )}
          </section>
        </aside>

        <section className="pane editor-pane">
          <div className="editor-head">
            <div className="segmented">
              {(["yaml", "preview", "split"] as ViewMode[]).map((item) => (
                <button className={mode === item ? "active" : ""} key={item} onClick={() => setMode(item)}>
                  {item === "yaml" ? "YAML" : item === "preview" ? "预览" : "分屏"}
                </button>
              ))}
            </div>
            <div className="status-line">
              {job?.status === "succeeded" ? <CheckCircle2 size={15} /> : null}
              <span>{message}</span>
            </div>
          </div>

          <div className={`editor-body mode-${mode}`}>
            {(mode === "yaml" || mode === "split") && (
              <textarea
                className="yaml-editor"
                value={yamlText}
                onChange={(event) => {
                  setYamlText(event.target.value);
                  setDraft(null);
                }}
                placeholder="生成后将在这里显示 YAML"
                spellCheck={false}
              />
            )}
            {(mode === "preview" || mode === "split") && (
              <ScriptPreview
                draft={parsedDraft}
                selectedSceneId={selectedScene?.id ?? ""}
                onSelectScene={setSelectedSceneId}
                characterName={characterName}
              />
            )}
          </div>
        </section>

        <aside className="pane inspector-pane">
          <section className="section">
            <div className="section-title">任务</div>
            <div className="progress-shell">
              <div className="progress-bar" style={{ width: `${job?.progress ?? 0}%` }} />
            </div>
            <div className="job-meta">
              <span>{job?.status ?? "idle"}</span>
              <span>{job?.progress ?? 0}%</span>
            </div>
            <p className="muted">{job?.current_step ?? "等待生成"}</p>
          </section>

          <section className="section grow">
            <div className="section-title">场景</div>
            {selectedScene ? (
              <SceneInspector scene={selectedScene} characterName={characterName} />
            ) : (
              <p className="muted">暂无场景</p>
            )}
          </section>

          <section className="section">
            <div className="section-title">重写</div>
            <textarea
              className="small-textarea"
              value={regenerateInstruction}
              onChange={(event) => setRegenerateInstruction(event.target.value)}
              spellCheck={false}
            />
            <button className="wide" onClick={regenerateScene} disabled={!selectedScene || busy}>
              <RefreshCcw size={16} />
              重写所选场景
            </button>
          </section>

          <section className="section">
            <div className="section-title">版本</div>
            <div className="version-list">
              {versions.length === 0 ? (
                <span className="muted">暂无版本</span>
              ) : (
                versions
                  .slice()
                  .reverse()
                  .map((version) => (
                    <div className="version-item" key={version.version_id}>
                      <div>
                        <strong>{version.version_id}</strong>
                        <span>{new Date(version.created_at).toLocaleString()}</span>
                      </div>
                      <div className="version-actions">
                        <button onClick={() => loadVersion(version.version_id)} disabled={busy}>
                          载入
                        </button>
                        <button onClick={() => restoreVersion(version.version_id)} disabled={busy}>
                          恢复
                        </button>
                      </div>
                    </div>
                  ))
              )}
            </div>
            <button className="wide ghost" onClick={loadSchema}>
              <FileText size={16} />
              查看 Schema
            </button>
            {schemaText ? <textarea className="schema-view" value={schemaText} readOnly /> : null}
          </section>
        </aside>
      </main>
    </div>
  );
}

function joinChapterText(chapters: Chapter[]) {
  return chapters.map((chapter) => `${chapter.title}\n${chapter.content ?? ""}`).join("\n\n");
}

function ScriptPreview({
  draft,
  selectedSceneId,
  onSelectScene,
  characterName
}: {
  draft: ScriptDraft | null;
  selectedSceneId: string;
  onSelectScene: (id: string) => void;
  characterName: (id: string) => string;
}) {
  if (!draft) {
    return <div className="empty-preview">等待生成结构化剧本</div>;
  }

  return (
    <div className="preview">
      <header className="preview-title">
        <h1>{draft.project.title}</h1>
        <p>{draft.world.logline}</p>
      </header>
      <div className="character-strip">
        {draft.characters.map((character) => (
          <span key={character.id}>
            {character.name} · {character.role}
          </span>
        ))}
      </div>
      {draft.acts.map((act) => (
        <section className="act-section" key={act.id}>
          <h2>{act.title}</h2>
          <p>{act.purpose}</p>
          {act.scene_ids
            .map((id) => draft.scenes.find((scene) => scene.id === id))
            .filter(Boolean)
            .map((scene) => (
              <button
                className={`scene-row ${selectedSceneId === scene!.id ? "selected" : ""}`}
                key={scene!.id}
                onClick={() => onSelectScene(scene!.id)}
              >
                <span>{scene!.id}</span>
                <strong>{scene!.location}</strong>
                <em>{scene!.summary}</em>
              </button>
            ))}
        </section>
      ))}
      <section className="act-section">
        <h2>对白片段</h2>
        {draft.scenes.slice(0, 4).flatMap((scene) =>
          (scene.dialogues ?? []).map((dialogue, index) => (
            <p className="dialogue-line" key={`${scene.id}-${index}`}>
              <strong>{characterName(dialogue.speaker)}</strong>
              <span>{dialogue.line}</span>
            </p>
          ))
        )}
      </section>
    </div>
  );
}

function SceneInspector({ scene, characterName }: { scene: Scene; characterName: (id: string) => string }) {
  return (
    <div className="scene-inspector">
      <div className="scene-kicker">
        <strong>{scene.id}</strong>
        <span>{scene.location}</span>
        <span>{scene.time}</span>
      </div>
      <h3>{scene.purpose}</h3>
      <p>{scene.summary}</p>
      <dl>
        <dt>来源章节</dt>
        <dd>{scene.chapter_refs.join(", ")}</dd>
        <dt>冲突</dt>
        <dd>{scene.conflict}</dd>
        <dt>角色</dt>
        <dd>{scene.characters.map(characterName).join(" / ")}</dd>
      </dl>
      <div className="beat-list">
        {scene.beats.map((beat, index) => (
          <span key={index}>{beat}</span>
        ))}
      </div>
      <div className="dialogue-list">
        {(scene.dialogues ?? []).map((dialogue, index) => (
          <p key={index}>
            <strong>{characterName(dialogue.speaker)}</strong>
            <span>{dialogue.line}</span>
          </p>
        ))}
      </div>
    </div>
  );
}

export default App;
