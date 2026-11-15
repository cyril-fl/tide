package listrecent

import (
	"context"
	"time"

	"github.com/cyril-fl/tide/core/services/secret"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/uptrace/bun"
)

// limit is how many notes the list shows.
const limit = 20

// Recent is a note and when it last changed.
type Recent struct {
	ID        string      `json:"id"`
	Title     secret.Text `json:"title" swaggertype:"string"`
	Path      string      `json:"path"`
	UpdatedAt time.Time   `json:"updated_at"`
	Readonly  bool        `json:"readonly,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
}

type cache struct {
	result []Recent
}

func Process(ctx context.Context, db bun.IDB) *run {
	return &run{
		ctx: ctx,
		db:  db,
	}
}

func (p *run) Run() {
	p.loadNotes()
}

// Getters & Setters

// GetResult returns the last changed notes, most recent first.
func (p *run) GetResult() []Recent {
	return p.cache.result
}

// Methods

// loadNotes falls back on created_at: a note never edited has no updated_at. A note under a trashed folder is not
// trashed itself: the tree drops it, and more are read until the list is full. Titles and paths come from the tree.
func (p *run) loadNotes() {
	tree, err := domain.LoadTree(p.ctx, p.db)
	if err != nil {
		p.SetErr(err)
		return
	}

	for read := limit; ; read *= 4 {
		var rows []Recent
		err := p.db.NewRaw(`
			SELECT i.id, coalesce(i.updated_at, i.created_at) AS updated_at, mn.readonly
			FROM items AS i
			JOIN metadata_notes AS mn ON mn.item_id = i.id
			WHERE i.user_id = ? AND i.kind = ? AND i.deleted_at IS NULL
			ORDER BY updated_at DESC, i.id
			LIMIT ?`,
			domain.UserID(p.ctx), domain.NOTE, read).
			Scan(p.ctx, &rows)
		if err != nil {
			p.SetErr(err)
			return
		}
		live := domain.Live(tree, rows, func(r Recent) string { return r.ID }, func(a, b Recent) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
		if len(live) >= limit || len(rows) < read {
			p.cache.result = domain.Page(live, limit, 0)
			break
		}
	}
	for i := range p.cache.result {
		recent := &p.cache.result[i]
		recent.Title, recent.Path = secret.Text(tree.Titles[recent.ID]), tree.Paths[recent.ID]
	}
}
