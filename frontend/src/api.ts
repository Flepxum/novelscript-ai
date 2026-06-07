import type { Chapter, Job, Project, ScriptResponse, VersionItem } from "./types";

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "";

type ApiError = {
  error?: {
    code: string;
    message: string;
    details?: Array<{ path: string; message: string }>;
  };
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {})
    }
  });
  const payload = (await response.json().catch(() => ({}))) as ApiError & { data?: T };
  if (!response.ok) {
    const details = payload.error?.details?.map((item) => `${item.path}: ${item.message}`).join("\n");
    throw new Error([payload.error?.message ?? "request failed", details].filter(Boolean).join("\n"));
  }
  return payload.data as T;
}

export const api = {
  health: () => request<{ status: string }>("/api/v1/health"),
  createProject: (body: { title: string; adaptation_target: string; language: string }) =>
    request<Project>("/api/v1/projects", {
      method: "POST",
      body: JSON.stringify(body)
    }),
  saveSource: (
    projectId: string,
    body: { novel_title: string; author: string; content: string }
  ) =>
    request<{ source_id: string; chapter_count: number; chapters: Chapter[] }>(
      `/api/v1/projects/${projectId}/source`,
      {
        method: "POST",
        body: JSON.stringify(body)
      }
    ),
  updateChapters: (projectId: string, chapters: Chapter[]) =>
    request<{ source_id: string; chapter_count: number; chapters: Chapter[] }>(
      `/api/v1/projects/${projectId}/chapters`,
      {
        method: "PUT",
        body: JSON.stringify({ chapters })
      }
    ),
  generate: (
    projectId: string,
    body: {
      style: string;
      target_scene_count: number;
      dialogue_density: string;
      preserve_original_names: boolean;
    }
  ) =>
    request<{ job_id: string; id: string; status: string; progress: number; current_step: string }>(
      `/api/v1/projects/${projectId}/generate`,
      {
        method: "POST",
        body: JSON.stringify(body)
      }
    ),
  getJob: (jobId: string) => request<Job>(`/api/v1/jobs/${jobId}`),
  getScript: (projectId: string) => request<ScriptResponse>(`/api/v1/projects/${projectId}/script`),
  saveScript: (projectId: string, yaml: string, editorNote: string) =>
    request<ScriptResponse>(`/api/v1/projects/${projectId}/script`, {
      method: "PUT",
      body: JSON.stringify({ yaml, editor_note: editorNote })
    }),
  regenerateScene: (projectId: string, sceneId: string, instruction: string) =>
    request<ScriptResponse & { scene: unknown }>(`/api/v1/projects/${projectId}/script/regenerate`, {
      method: "POST",
      body: JSON.stringify({ scope: "scene", scene_id: sceneId, instruction })
    }),
  listVersions: (projectId: string) => request<VersionItem[]>(`/api/v1/projects/${projectId}/script/versions`),
  schema: () => request<Record<string, unknown>>("/api/v1/schema/script")
};
