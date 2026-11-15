package readfork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

var ErrNotFork = errors.New("note is not a fork")

// Result is what a merge of the fork starts from: the note it goes back into, and the text both sides started from.
type Result struct {
	ForkOf string `json:"fork_of"`
	Base   string `json:"base"`
	// MergedAt is the version of the original the last merge saved, until that merge is undone: unmergefork's 409.
	MergedAt *time.Time `json:"merged_at,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	id  string
}

type cache struct {
	result Result
}

// Process reads the origin of a fork made by forknote, used by GET /api/notes/{id}/fork and mergefork.
func Process(ctx context.Context, db bun.IDB, id string) *run {
	return &run{
		ctx: ctx,
		db:  db,
		id:  id,
	}
}

func (p *run) Run() {
	p.loadNote()
	p.loadFork()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods

// loadNote reuses readitem: same errors (ErrNoValue, ErrNotFound) for a missing, trashed or foreign note.
func (p *run) loadNote() {
	process := readitem.Process(p.ctx, p.db, domain.NOTE, p.id)
	process.Run()
	p.SetErr(process.GetErr())
}

// loadFork needs no user_id check: loadNote has found the note of this account.
func (p *run) loadFork() {
	if p.HasErr() {
		return
	}

	var base secret.Text
	err := p.db.NewSelect().
		Model((*domain.MetadataNote)(nil)).
		Column("fork_of", "fork_base", "fork_merged_at").
		Where("item_id = ?", p.id).
		Scan(p.ctx, &p.cache.result.ForkOf, &base, &p.cache.result.MergedAt)
	p.cache.result.Base = string(base)
	if err == nil && p.cache.result.ForkOf == "" {
		err = fmt.Errorf("%w: %q", ErrNotFork, p.id)
	}
	p.SetErr(err)
}
