package forknote

import (
	"context"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

// Input is the note forked and the fork's text: a version of it or its current text, retitled by the app.
type Input struct {
	ID      string
	Content string
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	original readitem.Result
	fork     domain.Item
}

// Process creates a fork of a note beside it, used by POST /api/notes/{id}/forks: a note that remembers the note
// it came from, and its own first text as the base of a later merge (mergefork). The base is the fork's text, not
// the original's: a fork of the current text then needs no version, and base and fork share their title line.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

// Run reads the original, creates the fork and marks it in one transaction: a failure leaves no plain copy.
func (p *run) Run() {
	// p.ctx, not the one RunInTx hands: a bun.Tx's own ctx may act for another account.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(_ context.Context, tx bun.Tx) error {
		for _, step := range []func(bun.Tx) error{p.readOriginal, p.createFork, p.markFork} {
			if err := step(tx); err != nil {
				return err
			}
		}
		return nil
	}))
}

// Getters & Setters

// GetResult returns the fork, as POST /api/notes answers a note.
func (p *run) GetResult() domain.Item {
	return p.cache.fork
}

// Methods

// readOriginal reuses readitem: a missing, trashed or foreign note is readitem.ErrNotFound.
func (p *run) readOriginal(tx bun.Tx) error {
	process := readitem.Process(p.ctx, tx, domain.NOTE, p.input.ID)
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	p.cache.original = process.GetResult()
	return nil
}

// createFork puts the fork in the folder of the original.
func (p *run) createFork(tx bun.Tx) error {
	input := createnote.Input{Content: p.input.Content}
	if parent := p.cache.original.Item.ParentID; parent != nil {
		input.ParentID = *parent
	}
	process := createnote.Process(p.ctx, tx, input)
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	p.cache.fork = process.GetResult()
	return nil
}

// markFork keeps the fork's text as saved, its [[links]] turned into ids, so the first merge sees no change in them.
func (p *run) markFork(tx bun.Tx) error {
	process := readitem.Process(p.ctx, tx, domain.NOTE, p.cache.fork.ID)
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	base, err := secret.Seal(domain.UserID(p.ctx), process.GetResult().Content)
	if err != nil {
		return err
	}

	// createFork has made this note for this account: no user_id check needed here.
	_, err = tx.NewUpdate().
		Model((*domain.MetadataNote)(nil)).
		Set("fork_of = ?", p.cache.original.Item.ID).
		Set("fork_base = ?", base).
		Where("item_id = ?", p.cache.fork.ID).
		Exec(p.ctx)
	return err
}
