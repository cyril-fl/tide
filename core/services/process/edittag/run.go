package edittag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/cyril-fl/tide/core/services/linediff"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/secret"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

var (
	ErrNoValue       = errors.New("no tag value given")
	ErrNotFound      = errors.New("tag not found")
	ErrAlreadyExists = errors.New("tag already exists")

	// errDryRun makes the transaction of a dry run roll back; it never leaves the process.
	errDryRun = errors.New("dry run")
)

// Changed is a note whose text the rename rewrites, with the lines that change ("- old", "+ new").
type Changed struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Path  string   `json:"path"`
	Diff  []string `json:"diff"`
}

// uniqueViolation is the Postgres SQLSTATE raised here by ux_tags_value_lower.
const uniqueViolation = "23505"

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	from  string
	to    string
	merge bool
	dry   bool
}

type cache struct {
	from   string
	to     string
	result domain.Tag
	notes  []Changed
}

// Process renames the tag from as to. A to that is another tag already is refused, unless merge: from is then folded
// into it, its notes carrying to instead.
func Process(ctx context.Context, db bun.IDB, from string, to string, merge bool) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		from:  from,
		to:    to,
		merge: merge,
	}
}

// DryRun runs everything then rolls it back: the result is the one of a real call, GetNotes the notes it rewrote.
func (p *run) DryRun() *run {
	p.dry = true
	return p
}

func (p *run) Run() {
	p.makeValues()
	p.renameInTx()
}

// Getters & Setters

// GetResult returns the renamed tag.
func (p *run) GetResult() domain.Tag {
	return p.cache.result
}

// GetNotes returns, after a DryRun, the notes rewritten with their diff.
func (p *run) GetNotes() []Changed {
	return p.cache.notes
}

// Methods
func (p *run) makeValues() {
	p.cache.from = strings.TrimSpace(p.from)
	p.cache.to = strings.TrimSpace(p.to)

	switch {
	case p.cache.from == "" || p.cache.to == "":
		p.SetErr(ErrNoValue)
	case !hashtag.Valid(p.cache.to):
		p.SetErr(fmt.Errorf("%w: %q", hashtag.ErrInvalid, p.cache.to))
	}
}

// renameInTx renames the tag and its notes' text together: a failure changes neither.
func (p *run) renameInTx() {
	if p.HasErr() {
		return
	}

	err := p.db.RunInTx(p.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		merged, err := p.mergeTag(ctx, tx)
		if err == nil && !merged {
			if err = p.updateTag(ctx, tx); err == nil {
				err = p.rewriteNotes(ctx, tx, p.cache.result.ID)
			}
		}
		if err == nil && p.dry {
			err = errDryRun // the same steps ran, nothing stays
		}
		return err
	})
	if !errors.Is(err, errDryRun) {
		p.SetErr(err)
	}
}

// mergeTag folds from into the tag to when merge is asked and to is another tag: from's notes carry to, in their
// text too, then from is deleted. It tells whether it merged; to free or a case-only rename is left to updateTag.
// Checked before the update: a unique violation would abort the whole transaction.
func (p *run) mergeTag(ctx context.Context, tx bun.Tx) (bool, error) {
	if !p.merge {
		return false, nil
	}
	hashes, err := domain.BlindTags(p.ctx, []string{p.cache.from, p.cache.to})
	if err != nil {
		return false, err
	}
	var from domain.Tag
	err = tx.NewSelect().Model(&from).Where("user_id = ? AND value_hash = ?", domain.UserID(p.ctx), hashes[0]).Scan(ctx)
	if err != nil {
		return false, toDomainErr(err, p.cache.from, p.cache.to)
	}
	err = tx.NewSelect().Model(&p.cache.result).Where("user_id = ? AND value_hash = ? AND id <> ?", domain.UserID(p.ctx), hashes[1], from.ID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := p.rewriteNotes(ctx, tx, from.ID); err != nil {
		return false, err
	}
	_, err = tx.NewRaw(`INSERT INTO notes_on_tags (note_id, tag_id) SELECT note_id, ? FROM notes_on_tags WHERE tag_id = ?
		ON CONFLICT DO NOTHING`, p.cache.result.ID, from.ID).Exec(ctx)
	if err != nil {
		return false, err
	}
	// Its notes_on_tags rows go with it through ON DELETE CASCADE.
	_, err = tx.NewDelete().Model((*domain.Tag)(nil)).Where("id = ?", from.ID).Exec(ctx)
	return true, err
}

// updateTag matches by the blind of ux_tags_user_value_hash, whatever the case.
// A case-only rename (go -> Go) passes: the row does not conflict with itself.
func (p *run) updateTag(ctx context.Context, tx bun.Tx) error {
	from, err := secret.Blind(domain.UserID(p.ctx), p.cache.from)
	if err != nil {
		return err
	}
	to, err := domain.SealTag(p.ctx, p.cache.to)
	if err != nil {
		return err
	}
	err = tx.NewUpdate().
		Model((*domain.Tag)(nil)).
		Set("value = ?, value_hash = ?", to.Value, to.ValueHash).
		Where("user_id = ? AND value_hash = ?", domain.UserID(p.ctx), from).
		Returning("?Columns").
		Scan(ctx, &p.cache.result)

	return toDomainErr(err, p.cache.from, p.cache.to)
}

// rewriteNotes writes #to for #from in the notes of the tag tagID, each keeping its text before in its history.
// A note carrying both loses #from, not to hold #to twice. It also reaches the trash: a note restored later must not
// bring the old tag back.
func (p *run) rewriteNotes(ctx context.Context, tx bun.Tx, tagID string) error {
	var notes []domain.MetadataNote
	err := tx.NewSelect().
		Model(&notes).
		Column("item_id", "content").
		Join("JOIN notes_on_tags AS nt ON nt.note_id = mn.item_id").
		Where("nt.tag_id = ?", tagID).
		Scan(ctx)
	if err != nil {
		return err
	}

	for i := range notes {
		old := string(notes[i].Content)
		content := hashtag.Rename(old, p.cache.from, p.cache.to)
		if !strings.EqualFold(p.cache.from, p.cache.to) && slices.ContainsFunc(hashtag.Parse(old), func(tag string) bool { return strings.EqualFold(tag, p.cache.to) }) {
			content = hashtag.Remove(old, p.cache.from)
		}
		if content == old {
			continue
		}
		version, err := domain.NewNoteVersion(p.ctx, notes[i].ItemID, old)
		if err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(&version).Exec(ctx); err != nil {
			return err
		}
		if p.dry {
			read := readitem.Process(p.ctx, tx, domain.NOTE, notes[i].ItemID).SkipChildren()
			read.Run()
			if read.HasErr() {
				return read.GetErr()
			}
			r := read.GetResult()
			p.cache.notes = append(p.cache.notes, Changed{ID: r.Item.ID, Title: string(r.Item.Title), Path: r.Path, Diff: linediff.Lines(old, content)})
		}
		sealed, err := secret.Seal(domain.UserID(p.ctx), content)
		if err != nil {
			return err
		}
		if _, err := tx.NewUpdate().Model((*domain.MetadataNote)(nil)).Set("content = ?", sealed).Where("item_id = ?", notes[i].ItemID).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Helpers

// toDomainErr turns driver errors into the process errors callers can match with errors.Is.
func toDomainErr(err error, from string, to string) error {
	var pgErr pgdriver.Error

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: %q", ErrNotFound, from)
	case errors.As(err, &pgErr) && pgErr.Field('C') == uniqueViolation:
		return fmt.Errorf("%w: %q", ErrAlreadyExists, to)
	}

	return err
}
