package listpinned

import (
	"context"

	"github.com/cyril-fl/tide/core/services/secret"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/uptrace/bun"
)

// Pinned is a pinned note or a favorite folder.
type Pinned struct {
	ID       string      `json:"id"`
	Kind     domain.Kind `json:"kind"`
	Title    secret.Text `json:"title" swaggertype:"string"`
	Path     string      `json:"path"`
	Readonly bool        `json:"readonly,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
}

type cache struct {
	result []Pinned
}

func Process(ctx context.Context, db bun.IDB) *run {
	return &run{
		ctx: ctx,
		db:  db,
	}
}

// Run keeps live items only: a pinned note in the trash, or under a trashed folder, is not listed.
func (p *run) Run() {
	var rows []Pinned
	p.SetErr(p.db.NewRaw(`
		SELECT i.id, i.kind, i.title, COALESCE(mn.readonly, false) AS readonly
		FROM items AS i
		LEFT JOIN metadata_notes AS mn ON mn.item_id = i.id
		LEFT JOIN metadata_folders AS mf ON mf.item_id = i.id
		WHERE i.user_id = ? AND i.deleted_at IS NULL AND (mn.pinned OR mf.pinned)
		ORDER BY i.id`, domain.UserID(p.ctx)).
		Scan(p.ctx, &rows))
	if p.HasErr() {
		return
	}

	tree, err := domain.LoadTree(p.ctx, p.db)
	p.SetErr(err)
	p.cache.result = domain.Live(tree, rows, func(f Pinned) string { return f.ID }, nil)
	for i := range p.cache.result {
		p.cache.result[i].Path = tree.Paths[p.cache.result[i].ID]
	}
}

// Getters & Setters

// GetResult returns the pinned notes and favorite folders by path.
func (p *run) GetResult() []Pinned {
	return p.cache.result
}
