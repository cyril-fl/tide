package readtag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

var (
	ErrNoValue  = errors.New("no tag value given")
	ErrNotFound = errors.New("tag not found")
)

// Note is a note carrying the tag, with its path.
type Note struct {
	ID    string      `json:"id"`
	Title secret.Text `json:"title" swaggertype:"string"`
	Path  string      `json:"path"`
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

// loadNotes reads the ids of the tag's notes, keeps the live ones by path and names only the page: titles and paths
// come from the tree, no note is opened.
func (p *run) loadNotes() {
	if p.HasErr() {
		return
	}

	var ids []string
	p.SetErr(p.db.NewRaw(`
		SELECT i.id
		FROM notes_on_tags AS nt
		JOIN items AS i ON i.id = nt.note_id
		WHERE nt.tag_id = ? AND i.user_id = ? AND i.deleted_at IS NULL`, p.cache.result.Tag.ID, domain.UserID(p.ctx)).
		Scan(p.ctx, &ids))
	if p.HasErr() {
		return
	}

	tree, err := domain.LoadTree(p.ctx, p.db)
	p.SetErr(err)
	ids = domain.Live(tree, ids, func(id string) string { return id }, nil)
	p.cache.total = len(ids)
	for _, id := range domain.Page(ids, p.limit, p.offset) {
		p.cache.result.Notes = append(p.cache.result.Notes, Note{ID: id, Title: secret.Text(tree.Titles[id]), Path: tree.Paths[id]})
	}
}
