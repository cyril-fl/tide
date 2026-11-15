package mergefork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/deleteitems"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readfork"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

var ErrOriginGone = errors.New("the original note is in the trash or deleted")

// Input is a merge the app has made: the three-way merge and its conflicts are resolved there, by lines.
type Input struct {
	ID string // the fork
	// Content is the merged text, saved into the original.
	Content string
	// Base is the fork's text the merge started from: the base of the next one, so it never replays these changes.
	Base string
	// UpdatedAt is the version of the original the merge was made over: ErrStale once it changed since.
	UpdatedAt *time.Time
	// Trash moves the fork to the trash with the merge, like the branch deleted after a merge request.
	Trash bool
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	forkOf   string
	before   string // the original's text before the merge, put back by unmergefork
	original readitem.Result
}

// Process saves a merge of a fork into its original and moves the fork's base, used by POST /api/notes/{id}/merge.
// The original is saved as a restore: the text it replaces is kept in its history, the merge can be undone.
// A locked original is patchnote.ErrLocked, a changed one patchnote.ErrStale.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

// Run saves the original, the base and the trashed fork in one transaction: a base moved without its merge would lose those changes.
func (p *run) Run() {
	// p.ctx, not the one RunInTx hands: a bun.Tx's own ctx may act for another account.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(_ context.Context, tx bun.Tx) error {
		for _, step := range []func(bun.Tx) error{p.readFork, p.saveOriginal, p.moveBase, p.trashFork} {
			if err := step(tx); err != nil {
				return err
			}
		}
		return nil
	}))
}

// Getters & Setters

// GetResult returns the original after the merge, as GET /api/notes/{id} answers it.
func (p *run) GetResult() readitem.Result {
	return p.cache.original
}

// Methods

func (p *run) readFork(tx bun.Tx) error {
	process := readfork.Process(p.ctx, tx, p.input.ID)
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	p.cache.forkOf = process.GetResult().ForkOf
	return nil
}

func (p *run) saveOriginal(tx bun.Tx) error {
	read := readitem.Process(p.ctx, tx, domain.NOTE, p.cache.forkOf)
	read.Run()
	if errors.Is(read.GetErr(), readitem.ErrNotFound) {
		return fmt.Errorf("%w: %q", ErrOriginGone, p.cache.forkOf)
	}
	if read.HasErr() {
		return read.GetErr()
	}
	p.cache.before = read.GetResult().Content

	process := patchnote.Process(p.ctx, tx, patchnote.Input{ID: p.cache.forkOf, Content: &p.input.Content, UpdatedAt: p.input.UpdatedAt, Restore: true})
	process.Run()
	if errors.Is(process.GetErr(), readitem.ErrNotFound) {
		return fmt.Errorf("%w: %q", ErrOriginGone, p.cache.forkOf)
	}
	if process.HasErr() {
		return process.GetErr()
	}
	p.cache.original = process.GetResult()
	return nil
}

// moveBase keeps what unmergefork puts back, the base before (SET reads the old fork_base), the original's text
// before, its version after and whether the fork goes to the trash. No user_id check: readFork has found the fork of this account.
func (p *run) moveBase(tx bun.Tx) error {
	base, err := secret.Seal(domain.UserID(p.ctx), p.input.Base)
	if err != nil {
		return err
	}
	before, err := secret.Seal(domain.UserID(p.ctx), p.cache.before)
	if err != nil {
		return err
	}
	_, err = tx.NewUpdate().
		Model((*domain.MetadataNote)(nil)).
		Set("fork_undo_base = fork_base").
		Set("fork_undo_content = ?", before).
		Set("fork_merged_at = (SELECT coalesce(updated_at, created_at) FROM items WHERE id = ?)", p.cache.forkOf).
		Set("fork_base = ?", base).
		Set("fork_merged_trash = ?", p.input.Trash).
		Where("item_id = ?", p.input.ID).
		Exec(p.ctx)
	return err
}

// trashFork keeps its fork_* columns: unmergefork takes it out of the trash to undo the merge.
func (p *run) trashFork(tx bun.Tx) error {
	if !p.input.Trash {
		return nil
	}
	process := deleteitems.Process(p.ctx, tx, domain.NOTE, []string{p.input.ID})
	process.Run()
	if process.HasErr() {
		return process.GetErr()
	}
	return nil
}
