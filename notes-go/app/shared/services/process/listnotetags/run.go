package listnotetags

import (
	"context"

	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

// row is a tag put on a note, its value sealed.
type row struct {
	NoteID string
	Value  string
}

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	ids []string
}

type cache struct {
	result map[string][]string
}

// Process reads the tags of the notes of ids: a folder's children, a page of the CLI.
func Process(ctx context.Context, db bun.IDB, ids []string) *run {
	return &run{
		ctx: ctx,
		db:  db,
		ids: ids,
	}
}

func (p *run) Run() {
	p.loadTags()
}

// Getters & Setters

// GetResult returns the tags of each note by id, sorted; a note without tags is missing.
func (p *run) GetResult() map[string][]string {
	return p.cache.result
}

// Methods

func (p *run) loadTags() {
	p.cache.result = map[string][]string{}
	if len(p.ids) == 0 {
		return
	}

	var rows []row
	p.SetErr(p.db.NewRaw(`
		SELECT nt.note_id, t.value
		FROM notes_on_tags AS nt
		JOIN tags AS t ON t.id = nt.tag_id
		WHERE t.user_id = ? AND nt.note_id IN (?)`, domain.UserID(p.ctx), bun.In(p.ids)).
		Scan(p.ctx, &rows))
	if p.HasErr() {
		return
	}

	for i := range rows {
		var err error
		if rows[i].Value, err = secret.Open(rows[i].Value); err != nil {
			p.SetErr(err)
			return
		}
	}
	domain.SortFold(rows, func(r row) string { return r.Value }, nil)
	for _, r := range rows {
		p.cache.result[r.NoteID] = append(p.cache.result[r.NoteID], r.Value)
	}
}
