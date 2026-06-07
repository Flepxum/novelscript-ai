export type Chapter = {
  index: number;
  title: string;
  content?: string;
  word_count: number;
};

export type Project = {
  id: string;
  title: string;
  adaptation_target: string;
  language: string;
  created_at: string;
};

export type Job = {
  id: string;
  project_id: string;
  status: string;
  progress: number;
  current_step: string;
  error: string | null;
};

export type Dialogue = {
  speaker: string;
  line: string;
  parenthetical?: string;
  action?: string;
};

export type Character = {
  id: string;
  name: string;
  role: string;
  traits?: string[];
  goal?: string;
  conflict?: string;
  arc?: string;
};

export type Act = {
  id: string;
  title: string;
  purpose: string;
  scene_ids: string[];
};

export type Scene = {
  id: string;
  act_id: string;
  chapter_refs: number[];
  location: string;
  time: string;
  purpose: string;
  conflict: string;
  summary: string;
  characters: string[];
  beats: string[];
  dialogues?: Dialogue[];
  notes?: string[];
  ai_assumptions?: string[];
};

export type ScriptDraft = {
  schema_version: string;
  project: {
    title: string;
    adaptation_target: string;
    language: string;
    genre?: string[];
  };
  source: {
    novel_title?: string;
    author?: string;
    chapter_count: number;
    chapter_refs: number[];
  };
  world: {
    logline: string;
    theme: string[];
    tone: string;
    setting?: string;
  };
  characters: Character[];
  acts: Act[];
  scenes: Scene[];
  continuity: {
    timeline?: string[];
    open_threads?: string[];
    foreshadowing?: string[];
    props?: string[];
  };
  revision: {
    generated_at?: string;
    confidence?: string;
    editor_notes?: string[];
    ai_assumptions?: string[];
  };
};

export type ScriptResponse = {
  version_id: string;
  yaml: string;
  draft: ScriptDraft;
  created_at: string;
  editor_note?: string;
};

export type VersionItem = {
  version_id: string;
  created_at: string;
  editor_note?: string;
};

export type ValidationIssue = {
  path: string;
  message: string;
};
