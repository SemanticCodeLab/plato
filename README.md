# Plato

A simple, self-contained Markdown wiki server designed to be easy for **AI agents** and humans
to read and update.

- **Markdown files are the source of truth.** SQLite is only an index for page metadata, the
  cross-link graph, and API tokens.
- **Agent-friendly.** Update pages over a small REST API, or sync a whole directory of
  cross-linked Markdown with the `plato-sync` CLI.
- **Cross-links.** `[[wikilinks]]` and relative `.md` links are parsed and resolved into a typed
  link graph (`resolved` / `missing` / `ambiguous`), with backlinks.

## Quick start

```bash
# build
make build

# run the server (prints a one-time bootstrap token on first start)
./bin/plato -port 8080 -db ./plato.db -wiki-dir ./data
```

On first startup with no tokens, Plato prints a bootstrap token to stdout:

```
Plato bootstrap token:
plato_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Use it as `Authorization: Bearer plato_...` for all `/api/v1/*` calls.

### Create a wiki and a page

```bash
TOKEN=plato_xxxxxxxxxxxx

curl -s -XPOST localhost:8080/api/v1/wikis \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"slug":"demo","title":"Demo"}'

curl -s -XPOST localhost:8080/api/v1/wikis/demo/pages \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"title":"Authentication","rel_path":"Authentication.md","content":"# Authentication\n\nDepends on [[Database]].\n"}'
```

### Sync a directory of Markdown

```bash
./bin/plato-sync --dir ./docs --wiki demo --db ./plato.db --wiki-dir ./data [--delete]
```

## Web UI

```bash
cd web && npm install && npm run build
```

The built SPA is embedded into the `plato` binary and served at `/`.

## Flags

| Flag         | Default       | Meaning                          |
|--------------|---------------|----------------------------------|
| `-port`      | `8080`        | HTTP port                        |
| `-db`        | `./plato.db`  | SQLite database path             |
| `-wiki-dir`  | `./data`      | Root directory for Markdown      |

## License

MIT — see [LICENSE](LICENSE).
