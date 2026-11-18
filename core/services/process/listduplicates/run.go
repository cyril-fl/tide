package listduplicates

import (
	"context"
	"time"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/secret"
	"github.com/uptrace/bun"
)

// Note is a live note of a group, with the length of its content in characters.
type Note struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	UpdatedAt time.Time `json:"updated_at"`
	Length    int       `json:"length"`
}

// Group is the notes sharing a title (any case), or a folder and a title, hence a path: Key is that title or path.
type Group struct {
	Key   string `json:"key"`
	Notes []Note `json:"notes"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	byPath bool
}

type cache struct {
	rows   []row
	result []Group
}

// row is a note whose title another note has, in the base: the trash is dropped later, with the tree.
type row struct {
	ID        string
	ParentID  string
	TitleHash string
	UpdatedAt time.Time
}

// Process lists the groups of live notes with the same title, in any folder, or only with the same path (the same
// folder too) when byPath: the notes a path cannot tell apart. Untitled notes are not compared.
// TODO: notes with a near-identical content are not found, the content is sealed so the base cannot compare it; when an agent needs them, hash the content in noteindex.
func Process(ctx context.Context, db bun.IDB, byPath bool) *run {
	return &run{
		ctx:    ctx,
		db:     db,
		byPath: byPath,
	}
}

func (p *run) Run() {
	p.loadRows()
	p.group()
	p.loadLengths()
}

// Getters & Setters

// GetResult returns the groups by key, each with its notes by path then most recent first.
func (p *run) GetResult() []Group {
	return p.cache.result
}

// Methods

// loadRows keeps the notes whose title (and folder) another has: one pass, on the index of the title hashes.
func (p *run) loadRows() {
	partition := "user_id, title_hash"
	if p.byPath {
		partition += ", coalesce(parent_id, '00000000-0000-0000-0000-000000000000')"
	}
	p.SetErr(p.db.NewRaw(`
		SELECT id, coalesce(parent_id::text, '') AS parent_id, title_hash, coalesce(updated_at, created_at) AS updated_at
		FROM (
			SELECT *, count(*) OVER (PARTITION BY `+partition+`) AS same
			FROM items
			WHERE user_id = ? AND kind = ? AND deleted_at IS NULL AND title_hash IS NOT NULL
		) AS i
		WHERE same > 1`, domain.UserID(p.ctx), domain.NOTE).
		Scan(p.ctx, &p.cache.rows))
}

// group drops the notes of a trashed folder, which the tree does not know, then the groups left alone.
func (p *run) group() {
	if p.HasErr() || len(p.cache.rows) == 0 {
		return
	}
	tree, err := domain.LoadTree(p.ctx, p.db)
	if err != nil {
		p.SetErr(err)
		return
	}

	groups := map[string]*Group{}
	var order []*Group
	for _, r := range p.cache.rows {
		path, ok := tree.Paths[r.ID]
		if !ok {
			continue
		}
		id, key := r.TitleHash, tree.Titles[r.ID]
		if p.byPath {
			id, key = r.ParentID+"/"+r.TitleHash, path
		}
		g := groups[id]
		if g == nil {
			g = &Group{Key: key}
			groups[id], order = g, append(order, g)
		}
		g.Notes = append(g.Notes, Note{ID: r.ID, Path: path, UpdatedAt: r.UpdatedAt})
	}
	for _, g := range order {
		if len(g.Notes) > 1 {
			domain.SortFold(g.Notes, func(n Note) string { return n.Path }, func(a, b Note) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
			p.cache.result = append(p.cache.result, *g)
		}
	}
	domain.SortFold(p.cache.result, func(g Group) string { return g.Key }, nil)
}

// loadLengths opens the content of the notes left, the duplicates only: a handful.
func (p *run) loadLengths() {
	var ids []string
	for _, g := range p.cache.result {
		for _, n := range g.Notes {
			ids = append(ids, n.ID)
		}
	}
	if p.HasErr() || len(ids) == 0 {
		return
	}

	var rows []struct {
		ID      string      `bun:"id"`
		Content secret.Text `bun:"content"`
	}
	p.SetErr(p.db.NewRaw(`
		SELECT mn.item_id AS id, mn.content
		FROM metadata_notes AS mn
		JOIN items AS i ON i.id = mn.item_id
		WHERE i.user_id = ? AND mn.item_id::text IN (?)`, domain.UserID(p.ctx), bun.In(ids)).
		Scan(p.ctx, &rows))
	lengths := map[string]int{}
	for _, r := range rows {
		lengths[r.ID] = len([]rune(string(r.Content)))
	}
	for _, g := range p.cache.result {
		for i := range g.Notes {
			g.Notes[i].Length = lengths[g.Notes[i].ID]
		}
	}
}
