import { useEffect, useMemo, useState } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize from "rehype-sanitize";
import { useTheme, Theme } from "./theme";
import {
  api,
  ApiError,
  getToken,
  setToken,
  clearToken,
  Wiki,
  WikiStats,
  Page,
  PageWithContent,
  LinkView,
  Backlink,
  SourceType,
  BrokenLink,
} from "./api";

type View =
  | { name: "projects" }
  | { name: "newProject" }
  | { name: "pages"; wiki: string }
  | { name: "verify"; wiki: string }
  | { name: "page"; wiki: string; slug: string }
  | { name: "edit"; wiki: string; slug: string }
  | { name: "new"; wiki: string }
  | { name: "tokens" };

export default function App() {
  const [authed, setAuthed] = useState(!!getToken());
  const [view, setView] = useState<View>({ name: "projects" });

  if (!authed) return <Login onAuth={() => setAuthed(true)} />;

  return (
    <div className="app">
      <header>
        <h1 onClick={() => setView({ name: "projects" })} className="logo">
          Plato
        </h1>
        <nav>
          <button onClick={() => setView({ name: "projects" })}>Projects</button>
          <button onClick={() => setView({ name: "tokens" })}>Tokens</button>
          <ThemeControl />
          <button
            onClick={() => {
              clearToken();
              setAuthed(false);
            }}
          >
            Disconnect
          </button>
        </nav>
      </header>
      <main>
        {view.name === "projects" && <Projects nav={setView} />}
        {view.name === "newProject" && <NewProject nav={setView} />}
        {view.name === "pages" && <Pages wiki={view.wiki} nav={setView} />}
        {view.name === "verify" && <Verify wiki={view.wiki} nav={setView} />}
        {view.name === "page" && (
          <PageView wiki={view.wiki} slug={view.slug} nav={setView} />
        )}
        {view.name === "edit" && (
          <Editor wiki={view.wiki} slug={view.slug} nav={setView} />
        )}
        {view.name === "new" && <NewPage wiki={view.wiki} nav={setView} />}
        {view.name === "tokens" && <Tokens />}
      </main>
    </div>
  );
}

/* ---------- shared bits ---------- */

function ThemeControl() {
  const [theme, setTheme] = useTheme();
  const next: Record<Theme, Theme> = { system: "light", light: "dark", dark: "system" };
  const label: Record<Theme, string> = { system: "◐ System", light: "☀ Light", dark: "🌙 Dark" };
  return (
    <button title="Toggle theme" onClick={() => setTheme(next[theme])}>
      {label[theme]}
    </button>
  );
}

function useErr() {
  const [err, setErr] = useState("");
  return {
    err,
    setErr,
    run: async (fn: () => Promise<void>) => {
      try {
        setErr("");
        await fn();
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e));
      }
    },
  };
}

function Breadcrumbs({ trail }: { trail: { label: string; onClick?: () => void }[] }) {
  return (
    <nav className="crumbs">
      {trail.map((c, i) => (
        <span key={i}>
          {i > 0 && <span className="crumb-sep">/</span>}
          {c.onClick ? <a onClick={c.onClick}>{c.label}</a> : <span>{c.label}</span>}
        </span>
      ))}
    </nav>
  );
}

function Health({ stats }: { stats?: WikiStats }) {
  if (!stats) return null;
  return (
    <p className="health">
      <span>{stats.pages} pages</span>
      <span className="dot">·</span>
      <span>{stats.resolved} links</span>
      {stats.missing > 0 && (
        <>
          <span className="dot">·</span>
          <span className="link-missing">{stats.missing} missing</span>
        </>
      )}
      {stats.ambiguous > 0 && (
        <>
          <span className="dot">·</span>
          <span className="link-ambiguous">{stats.ambiguous} ambiguous</span>
        </>
      )}
    </p>
  );
}

function relTime(iso?: string): string {
  if (!iso) return "never";
  const t = new Date(iso).getTime();
  if (isNaN(t)) return "never";
  const s = Math.floor((Date.now() - t) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function sourceLabel(w: Wiki): string {
  switch (w.source_type) {
    case "git":
      return `git: ${w.source_url}${w.source_subdir ? " /" + w.source_subdir : ""}`;
    case "local":
      return `local: ${w.source_url}`;
    default:
      return "empty project";
  }
}

function PageMeta({ page }: { page: Page }) {
  const c = page.counts;
  return (
    <div className="row-meta">
      <code className="path">{page.rel_path}</code>
      {c && (
        <span className="counts">
          {c.outgoing > 0 && <span>{c.outgoing} links</span>}
          {c.backlinks > 0 && <span>{c.backlinks} backlinks</span>}
          {c.missing > 0 && <span className="link-missing">{c.missing} missing</span>}
          {c.ambiguous > 0 && <span className="link-ambiguous">{c.ambiguous} ambiguous</span>}
        </span>
      )}
    </div>
  );
}

/* ---------- login ---------- */

function Login({ onAuth }: { onAuth: () => void }) {
  useTheme(); // apply persisted theme on the login screen too
  const [token, setTok] = useState("");
  const [err, setErr] = useState("");
  return (
    <div className="login">
      <h1>Plato</h1>
      <p>Paste an API token (the bootstrap token is printed on first server start).</p>
      <input placeholder="plato_..." value={token} onChange={(e) => setTok(e.target.value)} />
      <button
        onClick={async () => {
          setToken(token.trim());
          try {
            await api.listWikis();
            onAuth();
          } catch {
            clearToken();
            setErr("Invalid token");
          }
        }}
      >
        Continue
      </button>
      {err && <p className="error">{err}</p>}
    </div>
  );
}

/* ---------- project dashboard ---------- */

function Projects({ nav }: { nav: (v: View) => void }) {
  const [projects, setProjects] = useState<Wiki[]>([]);
  const [loaded, setLoaded] = useState(false);
  const { err, run } = useErr();

  const load = () =>
    run(async () => {
      setProjects((await api.listWikis()).wikis || []);
      setLoaded(true);
    });
  useEffect(() => {
    load();
  }, []);

  return (
    <section>
      <Breadcrumbs trail={[{ label: "Plato" }, { label: "Projects" }]} />
      <div className="page-head">
        <h2>Projects</h2>
        <button onClick={() => nav({ name: "newProject" })}>+ New project</button>
      </div>
      {err && <p className="error">{err}</p>}

      {loaded && projects.length === 0 && (
        <div className="empty">
          <p>No projects yet.</p>
          <p className="muted">
            Add a local Markdown folder or Git repository to turn it into an
            agent-friendly wiki.
          </p>
          <button onClick={() => nav({ name: "newProject" })}>Add project</button>
        </div>
      )}

      <ul className="list">
        {projects.map((p) => (
          <li key={p.id} className="card-row">
            <div className="card-row-main">
              <a className="card-title" onClick={() => nav({ name: "pages", wiki: p.slug })}>
                {p.title}
              </a>
              <div className="row-meta">
                <code className="path">/{p.slug}</code>
              </div>
              <Health stats={p.stats} />
              <div className="row-meta">
                <span className="muted">{sourceLabel(p)}</span>
                <span className="dot">·</span>
                <span className="muted">indexed {relTime(p.last_indexed_at)}</span>
              </div>
            </div>
            <div className="card-row-actions">
              {p.source_type === "git" && (
                <PullButton wiki={p.slug} onDone={load} />
              )}
              <button onClick={() => nav({ name: "pages", wiki: p.slug })}>Open</button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

function PullButton({ wiki, onDone }: { wiki: string; onDone: () => void }) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  return (
    <span className="pull">
      <button
        disabled={busy}
        onClick={async () => {
          if (
            !confirm("Pulling updates may overwrite local project files. Continue?")
          )
            return;
          setBusy(true);
          setMsg("");
          try {
            const r = await api.gitPull(wiki);
            setMsg(`${r.pages_changed} changed`);
            onDone();
          } catch (e) {
            setMsg(e instanceof Error ? e.message : "pull failed");
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? "Pulling…" : "Pull updates"}
      </button>
      {msg && <span className="muted pull-msg">{msg}</span>}
    </span>
  );
}

/* ---------- new project (empty / local / git) ---------- */

function NewProject({ nav }: { nav: (v: View) => void }) {
  const [mode, setMode] = useState<SourceType>("empty");
  const [slug, setSlug] = useState("");
  const [title, setTitle] = useState("");
  const [localPath, setLocalPath] = useState("");
  const [gitUrl, setGitUrl] = useState("");
  const [branch, setBranch] = useState("");
  const [subdir, setSubdir] = useState("");
  const [busy, setBusy] = useState(false);
  const { err, run } = useErr();

  const create = () =>
    run(async () => {
      setBusy(true);
      try {
        const base = { slug, title: title || slug };
        if (mode === "empty") {
          await api.createProject({ ...base, source_type: "empty" });
        } else if (mode === "local") {
          await api.createProject({ ...base, source_type: "local", source_url: localPath });
        } else {
          await api.createProject({
            ...base,
            source_type: "git",
            source_url: gitUrl,
            source_branch: branch || undefined,
            source_subdir: subdir || undefined,
          });
        }
        nav({ name: "pages", wiki: slug.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") });
      } finally {
        setBusy(false);
      }
    });

  return (
    <section>
      <Breadcrumbs
        trail={[
          { label: "Plato", onClick: () => nav({ name: "projects" }) },
          { label: "Projects", onClick: () => nav({ name: "projects" }) },
          { label: "new" },
        ]}
      />
      <h2>New project</h2>

      <div className="tabs">
        <button className={mode === "empty" ? "tab active" : "tab"} onClick={() => setMode("empty")}>
          Empty
        </button>
        <button className={mode === "local" ? "tab active" : "tab"} onClick={() => setMode("local")}>
          Local folder
        </button>
        <button className={mode === "git" ? "tab active" : "tab"} onClick={() => setMode("git")}>
          Git repo
        </button>
      </div>

      {err && <p className="error">{err}</p>}

      <div className="card">
        <label>
          Project slug
          <input value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="semantic-cloud" />
        </label>
        <label>
          Project title
          <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Semantic Cloud" />
        </label>

        {mode === "local" && (
          <label>
            Local folder path
            <input
              value={localPath}
              onChange={(e) => setLocalPath(e.target.value)}
              placeholder="/home/you/docs/semantic-cloud"
            />
          </label>
        )}

        {mode === "git" && (
          <>
            <label>
              Git repository URL
              <input
                value={gitUrl}
                onChange={(e) => setGitUrl(e.target.value)}
                placeholder="https://github.com/example/docs.git"
              />
            </label>
            <label>
              Branch (optional, default main)
              <input value={branch} onChange={(e) => setBranch(e.target.value)} placeholder="main" />
            </label>
            <label>
              Subdirectory (optional)
              <input value={subdir} onChange={(e) => setSubdir(e.target.value)} placeholder="docs" />
            </label>
            <p className="preview muted">
              Public HTTPS repos only. SSH, file://, and credential URLs are rejected.
            </p>
          </>
        )}

        {mode === "empty" && (
          <p className="preview muted">Creates an empty project with a starter Home.md.</p>
        )}

        <button disabled={busy} onClick={create}>
          {busy ? "Creating…" : "Create project"}
        </button>
      </div>
    </section>
  );
}

/* ---------- page list (search + directory grouping) ---------- */

function dirOf(relPath: string): string {
  const i = relPath.lastIndexOf("/");
  return i < 0 ? "" : relPath.slice(0, i);
}

function Pages({ wiki, nav }: { wiki: string; nav: (v: View) => void }) {
  const [pages, setPages] = useState<Page[]>([]);
  const [stats, setStats] = useState<WikiStats | undefined>();
  const [q, setQ] = useState("");
  const { err, run } = useErr();

  useEffect(() => {
    run(async () => {
      setPages((await api.listPages(wiki)).pages || []);
      const w = (await api.listWikis()).wikis.find((x) => x.slug === wiki);
      setStats(w?.stats);
    });
  }, [wiki]);

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return pages;
    return pages.filter(
      (p) =>
        p.title.toLowerCase().includes(needle) ||
        p.slug.toLowerCase().includes(needle) ||
        p.rel_path.toLowerCase().includes(needle)
    );
  }, [pages, q]);

  const groups = useMemo(() => {
    const m = new Map<string, Page[]>();
    for (const p of filtered) {
      const d = dirOf(p.rel_path) || "/";
      if (!m.has(d)) m.set(d, []);
      m.get(d)!.push(p);
    }
    return [...m.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [filtered]);

  return (
    <section>
      <Breadcrumbs
        trail={[
          { label: "Plato", onClick: () => nav({ name: "projects" }) },
          { label: wiki },
        ]}
      />
      <div className="page-head">
        <h2>{wiki}</h2>
        <Health stats={stats} />
      </div>
      {err && <p className="error">{err}</p>}

      <div className="toolbar">
        <input
          className="search"
          placeholder="Search pages by title, slug, or path…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <button onClick={() => nav({ name: "verify", wiki })}>Verify links</button>
        <button onClick={() => nav({ name: "new", wiki })}>+ New page</button>
      </div>

      {groups.map(([dir, ps]) => (
        <div key={dir} className="group">
          <h3 className="group-head">
            <code>{dir}</code>
            <span className="muted"> {ps.length}</span>
          </h3>
          <ul className="list">
            {ps.map((p) => (
              <li key={p.id} className="row">
                <div>
                  <a onClick={() => nav({ name: "page", wiki, slug: p.slug })}>{p.title}</a>
                  <PageMeta page={p} />
                </div>
              </li>
            ))}
          </ul>
        </div>
      ))}
      {filtered.length === 0 && (
        <p className="muted">
          {pages.length === 0 ? "No pages yet." : "No pages match your search."}
        </p>
      )}
    </section>
  );
}

/* ---------- cross-link verification ---------- */

function Verify({ wiki, nav }: { wiki: string; nav: (v: View) => void }) {
  const [report, setReport] = useState<{ ok: boolean; broken: BrokenLink[] } | null>(
    null
  );
  const { err, run } = useErr();

  const load = () =>
    run(async () => {
      const r = await api.verify(wiki);
      setReport({ ok: r.ok, broken: r.broken || [] });
    });
  useEffect(() => {
    load();
  }, [wiki]);

  // Group broken links by source page.
  const byPage = useMemo(() => {
    const m = new Map<string, BrokenLink[]>();
    for (const b of report?.broken || []) {
      if (!m.has(b.from_rel_path)) m.set(b.from_rel_path, []);
      m.get(b.from_rel_path)!.push(b);
    }
    return [...m.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [report]);

  return (
    <section>
      <Breadcrumbs
        trail={[
          { label: "Plato", onClick: () => nav({ name: "projects" }) },
          { label: wiki, onClick: () => nav({ name: "pages", wiki }) },
          { label: "verify" },
        ]}
      />
      <div className="page-head">
        <h2>Cross-link verification</h2>
        <button onClick={load}>Re-check</button>
      </div>
      {err && <p className="error">{err}</p>}
      {!report && <p>Checking…</p>}
      {report?.ok && (
        <p className="notice">✓ All cross-references resolve. The graph is consistent.</p>
      )}
      {report && !report.ok && (
        <>
          <p className="error">
            {report.broken.length} broken cross-reference
            {report.broken.length === 1 ? "" : "s"} across {byPage.length} page
            {byPage.length === 1 ? "" : "s"}.
          </p>
          {byPage.map(([path, links]) => {
            const slug = links[0].from_slug;
            return (
              <div key={path} className="group">
                <h3 className="group-head">
                  <a onClick={() => nav({ name: "page", wiki, slug })}>{path}</a>
                  <span className="muted"> {links.length}</span>
                </h3>
                <ul className="links">
                  {links.map((b, i) => (
                    <li key={i} className={`link-${b.status}`}>
                      <code className="path">{b.raw}</code>
                      <span className="badge">
                        {b.kind} · {b.status}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            );
          })}
        </>
      )}
    </section>
  );
}

/* ---------- page viewer ---------- */

function statusClass(status: string) {
  return status === "resolved" ? "" : `link-${status}`;
}

function PageView({ wiki, slug, nav }: { wiki: string; slug: string; nav: (v: View) => void }) {
  const [page, setPage] = useState<PageWithContent | null>(null);
  const [outgoing, setOutgoing] = useState<LinkView[]>([]);
  const [backlinks, setBacklinks] = useState<Backlink[]>([]);
  const { err, run } = useErr();

  useEffect(() => {
    run(async () => {
      setPage(await api.getPage(wiki, slug));
      const l = await api.pageLinks(wiki, slug);
      setOutgoing(l.outgoing || []);
      setBacklinks(l.backlinks || []);
    });
  }, [wiki, slug]);

  if (err) return <p className="error">{err}</p>;
  if (!page) return <p>Loading…</p>;

  const missing = outgoing.filter((l) => l.status !== "resolved");

  return (
    <section>
      <Breadcrumbs
        trail={[
          { label: "Plato", onClick: () => nav({ name: "projects" }) },
          { label: wiki, onClick: () => nav({ name: "pages", wiki }) },
          { label: page.rel_path },
        ]}
      />
      <div className="page-toolbar">
        <button onClick={() => nav({ name: "edit", wiki, slug })}>Edit</button>
        <button
          className="danger"
          onClick={() =>
            run(async () => {
              if (confirm("Delete this page?")) {
                await api.deletePage(wiki, slug);
                nav({ name: "pages", wiki });
              }
            })
          }
        >
          Delete
        </button>
      </div>

      <h2 className="page-title">{page.title}</h2>
      <p className="page-sub">
        <code className="path">{page.rel_path}</code>
        <span className="dot">·</span>
        <span className="muted">{page.slug}</span>
        <span className="dot">·</span>
        <span className="muted hash">{page.content_hash}</span>
      </p>

      <article className="markdown">
        <Markdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
          {page.content}
        </Markdown>
      </article>

      <div className="link-zones">
        <div>
          <h3>Outgoing ({outgoing.length})</h3>
          <ul className="links">
            {outgoing.map((l, i) => (
              <li key={i} className={statusClass(l.status)}>
                {l.to_slug ? (
                  <a onClick={() => nav({ name: "page", wiki, slug: l.to_slug! })}>
                    {l.label || l.target}
                  </a>
                ) : (
                  <span>{l.label || l.target}</span>
                )}
                {l.status !== "resolved" && <span className="badge">{l.status}</span>}
              </li>
            ))}
            {outgoing.length === 0 && <li className="muted">none</li>}
          </ul>
        </div>
        <div>
          <h3>Backlinks ({backlinks.length})</h3>
          <ul className="links">
            {backlinks.map((b, i) => (
              <li key={i} className={statusClass(b.status)}>
                <a onClick={() => nav({ name: "page", wiki, slug: b.from_slug })}>{b.from_slug}</a>
              </li>
            ))}
            {backlinks.length === 0 && <li className="muted">none</li>}
          </ul>
        </div>
        {missing.length > 0 && (
          <div>
            <h3 className="link-missing">Missing / ambiguous ({missing.length})</h3>
            <ul className="links">
              {missing.map((l, i) => (
                <li key={i} className={statusClass(l.status)}>
                  {l.target} <span className="badge">{l.status}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </section>
  );
}

/* ---------- editor ---------- */

function Editor({ wiki, slug, nav }: { wiki: string; slug: string; nav: (v: View) => void }) {
  const [page, setPage] = useState<PageWithContent | null>(null);
  const [content, setContent] = useState("");
  const [baseHash, setBaseHash] = useState("");
  const [strict, setStrict] = useState(false);
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();

  useEffect(() => {
    run(async () => {
      const p = await api.getPage(wiki, slug);
      setPage(p);
      setContent(p.content);
      setBaseHash(p.content_hash);
    });
  }, [wiki, slug]);

  const save = async () => {
    setMsg("");
    try {
      const p = await api.updatePage(wiki, slug, content, baseHash, strict);
      setBaseHash(p.content_hash);
      setMsg("Saved.");
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        setMsg("This page changed since you opened it. Reload before saving.");
      } else if (e instanceof ApiError && e.status === 422) {
        const links = (e.body?.broken || []).map((b: any) => b.raw).join(", ");
        setMsg(`Rejected (strict): unresolved cross-links → ${links}`);
      } else {
        setMsg(e instanceof Error ? e.message : String(e));
      }
    }
  };

  return (
    <section>
      <Breadcrumbs
        trail={[
          { label: "Plato", onClick: () => nav({ name: "projects" }) },
          { label: wiki, onClick: () => nav({ name: "pages", wiki }) },
          { label: slug, onClick: () => nav({ name: "page", wiki, slug }) },
          { label: "edit" },
        ]}
      />
      {page && (
        <p className="page-sub">
          Editing: <strong>{page.title}</strong>
          <span className="dot">·</span>
          <code className="path">{page.rel_path}</code>
          <span className="dot">·</span>
          <span className="muted hash">{baseHash}</span>
        </p>
      )}
      <div className="page-toolbar">
        <label className="strict-toggle" title="Reject save if it introduces broken cross-links">
          <input type="checkbox" checked={strict} onChange={(e) => setStrict(e.target.checked)} />
          strict links
        </label>
        <button onClick={() => nav({ name: "page", wiki, slug })}>← Cancel</button>
        <button onClick={save}>Save</button>
      </div>
      {err && <p className="error">{err}</p>}
      {msg && <p className="notice">{msg}</p>}
      <textarea className="editor" value={content} onChange={(e) => setContent(e.target.value)} />
    </section>
  );
}

/* ---------- new page (path-first + slug preview) ---------- */

function slugify(s: string): string {
  return (
    s.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "page"
  );
}

function NewPage({ wiki, nav }: { wiki: string; nav: (v: View) => void }) {
  const [title, setTitle] = useState("");
  const [relPath, setRelPath] = useState("");
  const [content, setContent] = useState("# New page\n");
  const [strict, setStrict] = useState(false);
  const { err, setErr } = useErr();

  const effectivePath = relPath || `${slugify(title || "page")}.md`;
  const previewSlug = slugify(title || effectivePath.replace(/\.md$/, ""));

  return (
    <section>
      <Breadcrumbs
        trail={[
          { label: "Plato", onClick: () => nav({ name: "projects" }) },
          { label: wiki, onClick: () => nav({ name: "pages", wiki }) },
          { label: "new page" },
        ]}
      />
      <h2>New page in {wiki}</h2>
      {err && <p className="error">{err}</p>}
      <div className="card">
        <label>
          Title
          <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="My Page" />
        </label>
        <label>
          Path
          <input value={relPath} onChange={(e) => setRelPath(e.target.value)} placeholder="docs/my-page.md" />
        </label>
        <p className="preview">
          Slug: <code>{previewSlug}</code>
          <br />
          Will create: <code>data/{wiki}/{effectivePath}</code>
        </p>
      </div>
      <textarea className="editor" value={content} onChange={(e) => setContent(e.target.value)} />
      <div className="toolbar">
        <label className="strict-toggle" title="Reject if it introduces broken cross-links">
          <input type="checkbox" checked={strict} onChange={(e) => setStrict(e.target.checked)} />
          strict links
        </label>
        <button
          onClick={async () => {
            try {
              const p = await api.createPage(wiki, title, effectivePath, content, strict);
              nav({ name: "page", wiki, slug: p.slug });
            } catch (e) {
              if (e instanceof ApiError && e.status === 422) {
                const links = (e.body?.broken || []).map((b: any) => b.raw).join(", ");
                setErr(`Rejected (strict): unresolved cross-links → ${links}`);
              } else {
                setErr(e instanceof Error ? e.message : String(e));
              }
            }
          }}
        >
          Create
        </button>
      </div>
    </section>
  );
}

/* ---------- tokens ---------- */

function Tokens() {
  const [tokens, setTokens] = useState<
    { id: number; name: string; scopes: string[]; revoked: boolean }[]
  >([]);
  const [name, setName] = useState("");
  const [write, setWrite] = useState(true);
  const [fresh, setFresh] = useState("");
  const { err, run } = useErr();

  const load = () => run(async () => setTokens((await api.listTokens()).tokens || []));
  useEffect(() => {
    load();
  }, []);

  return (
    <section>
      <Breadcrumbs trail={[{ label: "Plato" }, { label: "Tokens" }]} />
      <h2>API tokens</h2>
      {err && <p className="error">{err}</p>}
      {fresh && (
        <p className="notice">
          New token (shown once): <code>{fresh}</code>
        </p>
      )}
      <div className="card">
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="local-agent" />
        </label>
        <label className="inline">
          <input type="checkbox" checked={write} onChange={(e) => setWrite(e.target.checked)} />
          write access
        </label>
        <button
          onClick={() =>
            run(async () => {
              const r = await api.createToken(name, write ? ["read", "write"] : ["read"]);
              setFresh(r.token);
              setName("");
              load();
            })
          }
        >
          Create token
        </button>
      </div>
      <ul className="list">
        {tokens.map((t) => (
          <li key={t.id} className="row">
            <span>
              {t.name} <span className="muted">[{t.scopes.join(", ")}]</span>
            </span>
            {t.revoked && <span className="badge">revoked</span>}
          </li>
        ))}
      </ul>
    </section>
  );
}
