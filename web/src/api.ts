// Client for the local findDevClean API.

export type Risk = "safe" | "regenerable" | "caution";

export interface Rule {
  id: string;
  name: string;
  category: string;
  risk: Risk;
  description: string;
}

export interface Finding {
  rule_id: string;
  name: string;
  category: string;
  risk: Risk;
  path: string;
  project_dir?: string;
  size: number;
  description: string;
}

export interface ScanResult {
  findings: Finding[];
  total: number;
  errors: string[];
}

export interface ScanProgress {
  phase: "walk" | "caches" | "done";
  done: number;
  total: number;
  current?: string;
}

interface ScanJob {
  id: string;
  state: "running" | "done" | "error";
  progress: ScanProgress;
  result?: ScanResult;
  error?: string;
}

export interface TreeNode {
  name: string;
  path?: string;
  is_dir: boolean;
  size: number;
  children?: TreeNode[];
}

interface TreeJob {
  id: string;
  state: "running" | "done";
  done: number;
  total: number;
  tree?: TreeNode;
  errors?: string[];
}

export type Status = "would_remove" | "removed" | "restored" | "rejected" | "failed";

export interface Outcome {
  path: string;
  size: number;
  status: Status;
  reason?: string;
}

export interface CleanReport {
  batch_id?: string;
  dry_run: boolean;
  freed: number;
  outcomes: Outcome[];
}

export interface Batch {
  id: string;
  created_at: string;
  entries: { original: string; quarantined: string; size: number }[];
}

const TOKEN_KEY = "findclean-token";

// Reads the token from the URL once, keeps it for this tab, and removes it
// from the address bar so it does not linger in history.
export function initToken(): void {
  const url = new URL(window.location.href);
  const fromUrl = url.searchParams.get("token");
  if (fromUrl) {
    try {
      sessionStorage.setItem(TOKEN_KEY, fromUrl);
    } catch {
      // Storage may be blocked; the token then lives only in memory below.
      memoryToken = fromUrl;
    }
    url.searchParams.delete("token");
    window.history.replaceState(null, "", url.pathname + url.search + url.hash);
  }
}

let memoryToken = "";

function token(): string {
  try {
    return sessionStorage.getItem(TOKEN_KEY) ?? memoryToken;
  } catch {
    return memoryToken;
  }
}

async function call<T>(path: string, method: "GET" | "POST", body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-Findclean-Token": token(),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const msg = (data as { error?: string }).error ?? `HTTP ${res.status}`;
    throw new Error(msg);
  }
  return data as T;
}

export const api = {
  rules: () => call<Rule[]>("/api/rules", "GET"),
  // Starts a scan and polls its progress until it finishes.
  scan: async (onProgress: (p: ScanProgress) => void): Promise<ScanResult> => {
    const { id } = await call<{ id: string }>("/api/scan", "POST");
    for (;;) {
      const job = await call<ScanJob>(`/api/scan/${id}`, "GET");
      onProgress(job.progress);
      if (job.state === "done" && job.result) return job.result;
      if (job.state === "error") throw new Error(job.error ?? "erro no escaneamento");
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
  },
  // Builds the disk map of the home folder and polls until it is ready.
  buildTree: async (onProgress: (p: { done: number; total: number }) => void): Promise<TreeNode> => {
    const { id } = await call<{ id: string }>("/api/tree", "POST");
    for (;;) {
      const job = await call<TreeJob>(`/api/tree/${id}`, "GET");
      onProgress({ done: job.done, total: job.total });
      if (job.state === "done" && job.tree) return job.tree;
      await new Promise((resolve) => setTimeout(resolve, 300));
    }
  },
  clean: (paths: string[], dryRun: boolean) =>
    call<CleanReport>("/api/clean", "POST", { paths, dry_run: dryRun }),
  batches: () => call<Batch[]>("/api/batches", "GET"),
  undo: (batchId: string) =>
    call<{ outcomes: Outcome[] }>("/api/undo", "POST", { batch_id: batchId }),
};
