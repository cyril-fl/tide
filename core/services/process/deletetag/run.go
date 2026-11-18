package deletetag

import (
	"context"
	"errors"
	"strings"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/secret"
	"github.com/uptrace/bun"
)

var (
	ErrNoValue = errors.New("no tag value given")

	// errDryRun makes the transaction of a dry run roll back; it never leaves the process.
	errDryRun = errors.New("dry run")
)

// Result is the tags deleted, the count of notes whose text changed (the trash included) and the notes sent to the
// trash for holding nothing else.
type Result struct {
	Tags    []domain.Tag `json:"tags"`
	Changed int          `json:"changed"`
	Trashed []Trashed    `json:"trashed"`
}

// Trashed is a note left empty by the deletion, sent to the trash.
type Trashed struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	values []string
	dry    bool
	keep   bool
}

type cache struct {
	values []string
	hashes []string // the value_hash of values
	result Result
	paths  map[string]string // the account's paths, loaded at the first note trashed
}

func Process(ctx context.Context, db bun.IDB, values []string) *run {
	return &run{
		ctx:    ctx,
		db:     db,
		values: values,
		cache:  cache{result: Result{Tags: []domain.Tag{}, Trashed: []Trashed{}}},
	}
}

// DryRun runs everything then rolls it back: the result is the one of a real call.
func (p *run) DryRun() *run {
	p.dry = true
	return p
}

// KeepEmpty leaves a note holding nothing but the tag empty, out of the trash.
func (p *run) KeepEmpty() *run {
	p.keep = true
	return p
}

func (p *run) Run() {
	p.makeValues()
	p.deleteInTx()
}

// Getters & Setters

// GetResult returns only the tags actually deleted, unknown values skipped, and the notes changed.
func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods
func (p *run) makeValues() {
	for _, value := range p.values {
		if value = strings.TrimSpace(value); value != "" {
			p.cache.values = append(p.cache.values, value)
		}
	}

	if len(p.cache.values) == 0 {
		p.SetErr(ErrNoValue)
		return
	}
	var err error
	p.cache.hashes, err = domain.BlindTags(p.ctx, p.cache.values)
	p.SetErr(err)
}

// deleteInTx erases the tags from their notes' text and deletes them together: a failure changes neither.
func (p *run) deleteInTx() {
	if p.HasErr() {
		return
	}

	err := p.db.RunInTx(p.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		err := p.rewriteNotes(ctx, tx)
		if err == nil {
			err = p.deleteTags(ctx, tx)
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

// rewriteNotes keeps a version of each note it changes. It also reaches the trash: a note restored later must not bring the tag back.
func (p *run) rewriteNotes(ctx context.Context, tx bun.Tx) error {
	var notes []domain.MetadataNote
	err := tx.NewSelect().
		Model(&notes).
		Column("item_id", "content").
		Where(`item_id IN (
			SELECT nt.note_id FROM notes_on_tags AS nt JOIN tags AS t ON t.id = nt.tag_id
			WHERE t.user_id = ? AND t.value_hash IN (?))`, domain.UserID(p.ctx), bun.In(p.cache.hashes)).
		Scan(ctx)
	if err != nil {
		return err
	}

	for i := range notes {
		old := string(notes[i].Content)
		content := hashtag.Remove(old, p.cache.values...)
		if content == old {
			continue
		}
		p.cache.result.Changed++
		// Its text before stays in its history: nothing else holds it.
		version, err := domain.NewNoteVersion(p.ctx, notes[i].ItemID, old)
		if err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(&version).Exec(ctx); err != nil {
			return err
		}
		sealed, err := secret.Seal(domain.UserID(p.ctx), content)
		if err != nil {
			return err
		}
		if _, err := tx.NewUpdate().Model((*domain.MetadataNote)(nil)).Set("content = ?", sealed).Where("item_id = ?", notes[i].ItemID).Exec(ctx); err != nil {
			return err
		}
		if content == "" && !p.keep {
			if err := p.trashNote(ctx, tx, notes[i].ItemID); err != nil {
				return err
			}
		}
	}
	return nil
}

// trashNote is for a note that held nothing but the tag: an empty note goes to the trash. One in the trash already,
// itself or through its folder, is left as it is: a new deleted_at would part it from what went there with it.
func (p *run) trashNote(ctx context.Context, tx bun.Tx, id string) error {
	if p.cache.paths == nil {
		paths, err := domain.Paths(p.ctx, tx)
		if err != nil {
			return err
		}
		p.cache.paths = paths
	}
	read := readitem.Process(p.ctx, tx, domain.NOTE, id).WithPaths(p.cache.paths)
	read.Run()
	if errors.Is(read.GetErr(), readitem.ErrNotFound) {
		return nil
	}
	if read.HasErr() {
		return read.GetErr()
	}
	r := read.GetResult()
	p.cache.result.Trashed = append(p.cache.result.Trashed, Trashed{ID: r.Item.ID, Title: string(r.Item.Title), Path: r.Path})

	_, err := tx.NewUpdate().
		Model((*domain.Item)(nil)).
		Set("deleted_at = current_timestamp").
		Where("id = ? AND user_id = ?", id, domain.UserID(p.ctx)).
		Exec(ctx)
	return err
}

// deleteTags matches by the blinds of ux_tags_user_value_hash, whatever the case.
// notes_on_tags rows go with it through ON DELETE CASCADE.
func (p *run) deleteTags(ctx context.Context, tx bun.Tx) error {
	return tx.NewDelete().
		Model((*domain.Tag)(nil)).
		Where("user_id = ? AND value_hash IN (?)", domain.UserID(p.ctx), bun.In(p.cache.hashes)).
		Returning("?Columns").
		Scan(ctx, &p.cache.result.Tags)
}
