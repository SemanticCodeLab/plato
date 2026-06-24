package sync

import (
	"encoding/json"

	"github.com/plato/plato/internal/store"
	"github.com/plato/plato/internal/wiki"
)

// Report summarizes a sync run.
type Report struct {
	Created   []string `json:"created"`
	Updated   []string `json:"updated"`
	Unchanged []string `json:"unchanged"`
	Deleted   []string `json:"deleted"`
}

// Run imports all *.md under dir into the given wiki via the service. When
// withDelete is true, live pages whose source file vanished are soft-deleted.
// actor is recorded in the audit log.
func Run(svc *wiki.Service, w *store.Wiki, dir string, withDelete bool, actor string) (*Report, error) {
	files, err := Walk(dir)
	if err != nil {
		return nil, err
	}
	rep := &Report{}
	seen := make(map[string]bool, len(files))

	for _, f := range files {
		content, err := ReadSource(f.AbsPath)
		if err != nil {
			return nil, err
		}
		p, action, err := svc.SyncPage(w, f.RelPath, content)
		if err != nil {
			return nil, err
		}
		seen[p.RelPath] = true
		switch action {
		case "created":
			rep.Created = append(rep.Created, p.RelPath)
		case "updated":
			rep.Updated = append(rep.Updated, p.RelPath)
		default:
			rep.Unchanged = append(rep.Unchanged, p.RelPath)
		}
	}

	if withDelete {
		live, err := svc.LivePageRelPaths(w)
		if err != nil {
			return nil, err
		}
		for rel := range live {
			if !seen[rel] {
				if err := svc.DeletePage(w, live[rel].Slug); err != nil {
					return nil, err
				}
				rep.Deleted = append(rep.Deleted, rel)
			}
		}
	}

	// Re-resolve so newly created pages satisfy previously-missing links.
	if err := svc.ResolveWikiLinks(w); err != nil {
		return nil, err
	}

	meta, _ := json.Marshal(rep)
	_ = svc.DB.Audit(actor, "sync.run", w.ID, 0, string(meta))
	return rep, nil
}
