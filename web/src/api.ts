// Minimal typed client for the Plato REST API. The token is kept in localStorage.

const TOKEN_KEY = "plato_token";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || "";
}
export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t);
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export interface WikiStats {
  pages: number;
  resolved: number;
  missing: number;
  ambiguous: number;
}
export type SourceType = "empty" | "local" | "git";

export interface Wiki {
  id: number;
  slug: string;
  title: string;
  created_at: string;
  source_type: SourceType;
  source_url?: string;
  source_branch?: string;
  source_subdir?: string;
  last_indexed_at?: string;
  stats?: WikiStats;
}

export interface CreateProjectReq {
  slug: string;
  title: string;
  source_type: SourceType;
  source_url?: string;
  source_branch?: string;
  source_subdir?: string;
}
export interface PageCounts {
  outgoing: number;
  backlinks: number;
  missing: number;
  ambiguous: number;
}
export interface Page {
  id: number;
  wiki_id: number;
  slug: string;
  title: string;
  rel_path: string;
  content_hash: string;
  counts?: PageCounts;
}
export interface PageWithContent extends Page {
  content: string;
}
export interface LinkView {
  raw: string;
  target: string;
  label?: string;
  kind: string;
  status: string;
  origin: string;
  to_slug?: string;
}

export interface GraphNode {
  id: number;
  slug: string;
  title: string;
  rel_path: string;
  outgoing: number;
  backlinks: number;
}
export interface GraphEdge {
  from: number;
  to: number;
  kind: string;
  origin: string;
}
export interface Graph {
  wiki: string;
  nodes: GraphNode[];
  edges: GraphEdge[];
}
export interface Backlink {
  from_slug: string;
  raw: string;
  status: string;
}

export class ApiError extends Error {
  status: number;
  body: any;
  constructor(status: number, body: any) {
    super(body?.error || `HTTP ${status}`);
    this.status = status;
    this.body = body;
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: "Bearer " + getToken(),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new ApiError(res.status, data);
  return data as T;
}

export interface BrokenLink {
  from_slug: string;
  from_rel_path: string;
  raw: string;
  target: string;
  kind: string;
  status: string;
}
export interface VerifyReport {
  ok: boolean;
  stats: WikiStats;
  broken: BrokenLink[];
}

export const api = {
  listWikis: () => req<{ wikis: Wiki[] }>("GET", "/api/v1/wikis"),
  createProject: (body: CreateProjectReq) =>
    req<Wiki>("POST", "/api/v1/wikis", body),
  gitPull: (wiki: string) =>
    req<{ ok: boolean; pages_changed: number; links_reindexed: number }>(
      "POST",
      `/api/v1/wikis/${wiki}/git/pull`
    ),
  listPages: (wiki: string) =>
    req<{ pages: Page[] }>("GET", `/api/v1/wikis/${wiki}/pages`),
  getPage: (wiki: string, slug: string) =>
    req<PageWithContent>("GET", `/api/v1/wikis/${wiki}/pages/${slug}`),
  createPage: (
    wiki: string,
    title: string,
    rel_path: string,
    content: string,
    strict = false
  ) =>
    req<Page>(
      "POST",
      `/api/v1/wikis/${wiki}/pages${strict ? "?strict=1" : ""}`,
      { title, rel_path, content }
    ),
  updatePage: (
    wiki: string,
    slug: string,
    content: string,
    base_hash: string,
    strict = false
  ) =>
    req<Page>(
      "PUT",
      `/api/v1/wikis/${wiki}/pages/${slug}${strict ? "?strict=1" : ""}`,
      { content, base_hash }
    ),
  verify: (wiki: string) => req<VerifyReport>("GET", `/api/v1/wikis/${wiki}/verify`),
  graph: (wiki: string) => req<Graph>("GET", `/api/v1/wikis/${wiki}/graph`),
  addLink: (
    wiki: string,
    pageSlug: string,
    body: { to?: string; to_path?: string; to_title?: string; label?: string }
  ) =>
    req<{ outgoing: LinkView[] }>(
      "POST",
      `/api/v1/wikis/${wiki}/pages/${pageSlug}/links`,
      body
    ),
  removeLink: (wiki: string, pageSlug: string, raw: string) =>
    req<{ removed: boolean }>(
      "DELETE",
      `/api/v1/wikis/${wiki}/pages/${pageSlug}/links`,
      { raw }
    ),
  deletePage: (wiki: string, slug: string) =>
    req<{ deleted: boolean }>("DELETE", `/api/v1/wikis/${wiki}/pages/${slug}`),
  pageLinks: (wiki: string, slug: string) =>
    req<{ outgoing: LinkView[]; backlinks: Backlink[] }>(
      "GET",
      `/api/v1/wikis/${wiki}/pages/${slug}/links`
    ),
  listTokens: () =>
    req<{ tokens: { id: number; name: string; scopes: string[]; revoked: boolean }[] }>(
      "GET",
      "/api/v1/tokens"
    ),
  createToken: (name: string, scopes: string[]) =>
    req<{ id: number; name: string; token: string; scopes: string[] }>(
      "POST",
      "/api/v1/tokens",
      { name, scopes }
    ),
};
