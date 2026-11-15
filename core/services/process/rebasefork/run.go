package rebasefork

import (
	"context"
	"time"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/readfork"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/secret"
	"github.com/uptrace/bun"
)

// Input is a rebase the app has made: the three-way merge and its conflicts are resolved there, by lines.
type Input struct {
	ID string // the fork
	// Content is the rebased text, saved into the fork.
	Content string
	// Base is the original's text the rebase started from: the base of the next merge, so it never replays these changes.
	Base string
	// UpdatedAt is the version of the fork the rebase was made over: ErrStale once it changed since.
	UpdatedAt *time.Time
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	fork readitem.Result
}

// Process saves into a fork what changed in its original and moves the fork's base, used by POST /api/notes/{id}/rebase.
// The fork is saved as a restore: the text it replaces is kept in its history, the rebase can be undone.
// A locked fork is patchnote.ErrLocked, a changed one patchnote.ErrStale.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

// Run saves the fork and the base in one transaction: a base moved without its text would replay those changes.
func (p *run) Run() {
	// p.ctx, not the one RunInTx hands: a bun.Tx's own ctx may act for another account.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(_ context.Context, tx bun.Tx) error {
		for _, step := range []func(bun.Tx) error{p.readFork, p.saveFork, p.moveBase} {
			if err := step(tx); err != nil {
				return err
			}
		}
		return nil
	}))
}

// Getters & Setters

// GetResult returns the fork after the rebase, as GET /api/notes/{id} answers it.
func (p *run) GetResult() readitem.Result {
	return p.cache.fork
}

// Methods

// readFork refuses a note that is no fork: readfork.ErrNotFork.
func (p *run) readFork(tx bun.Tx) error {
	process := readfork.Process(p.ctx, tx, p.input.ID)
	process.Run()
	return process.GetErr()
}

func (p *run) saveFork(tx bun.Tx) error {
	process := patchnote.Process(p.ctx, tx, patchnote.Input{ID: p.input.ID, Content: &p.input.Content, UpdatedAt: p.input.UpdatedAt, Restore: true})
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	p.cache.fork = process.GetResult()
	return nil
}

// moveBase also drops the last merge's undo: unmergefork would put back a base from before the rebase, and the next
// merge would replay what the fork took in. No user_id check: readFork has found the fork of this account.
func (p *run) moveBase(tx bun.Tx) error {
	base, err := secret.Seal(domain.UserID(p.ctx), p.input.Base)
	if err != nil {
		return err
	}
	_, err = tx.NewUpdate().
		Model((*domain.MetadataNote)(nil)).
		Set("fork_base = ?", base).
		Set("fork_undo_base = NULL, fork_undo_content = NULL, fork_merged_at = NULL, fork_merged_trash = NULL").
		Where("item_id = ?", p.input.ID).
		Exec(p.ctx)
	return err
}
