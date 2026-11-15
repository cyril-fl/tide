package readtag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cyril-fl/tide/core/services/secret"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/uptrace/bun"
)

var (
	ErrNoValue  = errors.New("no tag value given")
	ErrNotFound = errors.New("tag not found")
)

// Note is a note carrying the tag, with its path, its dates and its lock as a folder lists them.
type Note struct {
	ID        string      `json:"id"`
	Title     secret.Text `json:"title" swaggertype:"string"`
	Path      string      `json:"path"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at,omitzero" binding:"optional"`
	Readonly  bool        `json:"readonly,omitempty" binding:"optional"`
}

// Result is the tag and its live notes, by path.
type Result struct {
	Tag   domain.Tag `json:"tag"`
	Notes []Note     `json:"notes"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	value  string
	limit  int // 0: every note
	offset int
}

type cache struct {
	value  string
	result Result
	total  int
}

func Process(ctx context.Context, db bun.IDB, value string) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		value: value,
	}
}

// Page keeps limit notes from offset, in the same order; GetTotal still counts them all.
func (p *run) Page(limit, offset int) *run {
	p.limit, p.offset = limit, offset
	return p
}

func (p *run) Run() {
	p.makeValue()
	p.loadTag()
	p.loadNotes()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

func (p *run) GetTotal() int {
	return p.cache.total
}

// Methods

func (p *run) makeValue() {
	p.cache.value = strings.TrimSpace(p.value)
	if p.cache.value == "" {
		p.SetErr(ErrNoValue)
	}
}

// loadTag matches by the blind of ux_tags_user_value_hash, whatever the case.
func (p *run) loadTag() {
	if p.HasErr() {
		return
	}

	hash, err := secret.Blind(domain.UserID(p.ctx), p.cache.value)
	if err != nil {
		p.SetErr(err)
		return
	}
	err = p.db.NewSelect().
		Model(&p.cache.result.Tag).
		Where("user_id = ? AND value_hash = ?", domain.UserID(p.ctx), hash).
		Scan(p.ctx)
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("%w: %q", ErrNotFound, p.cache.value)
	}
	p.SetErr(err)
}

// loadNotes reads the tag's notes (dates, lock), keeps the live ones by path and names only the page: titles and paths
// come from the tree, no note is opened.
func (p *run) loadNotes() {
	if p.HasErr() {
		return
	}

	var notes []Note
	p.SetErr(p.db.NewRaw(`
		SELECT i.id, i.created_at, i.updated_at,
			COALESCE((SELECT mn.readonly FROM metadata_notes AS mn WHERE mn.item_id = i.id), false) AS readonly
		FROM notes_on_tags AS nt
		JOIN items AS i ON i.id = nt.note_id
		WHERE nt.tag_id = ? AND i.user_id = ? AND i.deleted_at IS NULL`, p.cache.result.Tag.ID, domain.UserID(p.ctx)).
		Scan(p.ctx, &notes))
	if p.HasErr() {
		return
	}

	tree, err := domain.LoadTree(p.ctx, p.db)
	p.SetErr(err)
	notes = domain.Live(tree, notes, func(note Note) string { return note.ID }, nil)
	p.cache.total = len(notes)
	for _, note := range domain.Page(notes, p.limit, p.offset) {
		note.Title, note.Path = secret.Text(tree.Titles[note.ID]), tree.Paths[note.ID]
		p.cache.result.Notes = append(p.cache.result.Notes, note)
	}
}
