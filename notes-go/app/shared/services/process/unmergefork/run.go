package unmergefork

import (
	"context"
	"errors"
	"fmt"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/mergefork"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readfork"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

var ErrNoMerge = errors.New("the fork has no merge to undo")

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	id  string  // the fork
}

type cache struct {
	origin   readfork.Result
	content  string // the original's text before the merge
	original readitem.Result
}

// Process undoes the last merge of a fork, used by POST /api/notes/{id}/unmerge, like git reset --hard HEAD~1: the
// original gets back its text from before it, saved as a restore, and the fork its base, so the next merge offers
// again what this one refused. One level only. An original changed since the merge is patchnote.ErrStale, its
// text is not overwritten; a locked one patchnote.ErrLocked.
func Process(ctx context.Context, db bun.IDB, id string) *run {
	return &run{
		ctx: ctx,
		db:  db,
		id:  id,
	}
}

// Run restores the original and the base in one transaction: a base moved back without its original would replay the merge.
func (p *run) Run() {
	// p.ctx, not the one RunInTx hands: a bun.Tx's own ctx may act for another account.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(_ context.Context, tx bun.Tx) error {
		for _, step := range []func(bun.Tx) error{p.readFork, p.restoreOriginal, p.restoreBase} {
			if err := step(tx); err != nil {
				return err
			}
		}
		return nil
	}))
}

// Getters & Setters

// GetResult returns the original after the undo, as GET /api/notes/{id} answers it.
func (p *run) GetResult() readitem.Result {
	return p.cache.original
}

// Methods

// readFork needs no user_id check on its second query: readfork has found the fork of this account.
func (p *run) readFork(tx bun.Tx) error {
	process := readfork.Process(p.ctx, tx, p.id)
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	p.cache.origin = process.GetResult()
	if p.cache.origin.MergedAt == nil {
		return fmt.Errorf("%w: %q", ErrNoMerge, p.id)
	}

	var content secret.Text
	err := tx.NewSelect().
		Model((*domain.MetadataNote)(nil)).
		Column("fork_undo_content").
		Where("item_id = ?", p.id).
		Scan(p.ctx, &content)
	p.cache.content = string(content)
	return err
}

func (p *run) restoreOriginal(tx bun.Tx) error {
	forkOf := p.cache.origin.ForkOf
	process := patchnote.Process(p.ctx, tx, patchnote.Input{ID: forkOf, Content: &p.cache.content, UpdatedAt: p.cache.origin.MergedAt, Restore: true})
	process.Run()
	if errors.Is(process.GetErr(), readitem.ErrNotFound) {
		return fmt.Errorf("%w: %q", mergefork.ErrOriginGone, forkOf)
	}
	if process.HasErr() {
		return process.GetErr()
	}
	p.cache.original = process.GetResult()
	return nil
}

// restoreBase needs no user_id check: readFork has found the fork of this account.
func (p *run) restoreBase(tx bun.Tx) error {
	_, err := tx.NewUpdate().
		Model((*domain.MetadataNote)(nil)).
		Set("fork_base = fork_undo_base").
		Set("fork_undo_base = NULL, fork_undo_content = NULL, fork_merged_at = NULL").
		Where("item_id = ?", p.id).
		Exec(p.ctx)
	return err
}
