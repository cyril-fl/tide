package tagnotes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/process/resolveitem"
	"github.com/uptrace/bun"
)

// ErrUnread is a tag written in the text where hashtag.Parse would not read it: the note is left as it was.
var ErrUnread = errors.New("the tag would not be read where it is written")

// Input names the notes by uuid or path from the root (/Folder/Note), and the tags without their #.
type Input struct {
	Notes  []string `json:"notes"`
	Add    []string `json:"add,omitempty" binding:"optional"`
	Remove []string `json:"remove,omitempty" binding:"optional"`
}

// Tagged is a note after the call, its content left out, or why it was left as it was.
type Tagged struct {
	Note  string   `json:"note"` // the uuid or path asked
	ID    string   `json:"id,omitempty" binding:"optional"`
	Title string   `json:"title,omitempty" binding:"optional"`
	Path  string   `json:"path,omitempty" binding:"optional"`
	Tags  []string `json:"tags,omitempty" binding:"optional"`
	Error string   `json:"error,omitempty" binding:"optional"`
	// What the call did with each tag asked, against the tags the note had before (the case ignored).
	Added          []string `json:"added,omitempty" binding:"optional"`
	AlreadyPresent []string `json:"already_present,omitempty" binding:"optional"`
	Removed        []string `json:"removed,omitempty" binding:"optional"`
	NotPresent     []string `json:"not_present,omitempty" binding:"optional"`
}

// diff sorts the tags asked into those the note had (before) and those it had not.
func diff(before, asked []string) (had, hadNot []string) {
	for _, tag := range asked {
		if slices.ContainsFunc(before, func(b string) bool { return strings.EqualFold(b, tag) }) {
			had = append(had, tag)
		} else {
			hadNot = append(hadNot, tag)
		}
	}
	return had, hadNot
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	result []Tagged
}

// Process adds and removes #tags on notes in one transaction, the rest of their text kept as it is: a note missing or
// locked is left with its error, the others tagged; each note changed keeps a version of its text before.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.checkInput()
	p.tagNotes()
}

// Getters & Setters

func (p *run) GetResult() []Tagged {
	return p.cache.result
}

// Methods

func (p *run) checkInput() {
	for _, tag := range p.input.Add {
		if !hashtag.Valid(tag) {
			p.SetErr(fmt.Errorf("%w: %q", hashtag.ErrInvalid, tag))
			return
		}
		if slices.ContainsFunc(p.input.Remove, func(r string) bool { return strings.EqualFold(r, tag) }) {
			p.SetErr(fmt.Errorf("%w: add and remove: %s in both", hashtag.ErrInvalid, tag))
			return
		}
	}
}

func (p *run) tagNotes() {
	if p.HasErr() {
		return
	}

	p.cache.result = make([]Tagged, 0, len(p.input.Notes))
	// p.ctx, not the one RunInTx hands: a bun.Tx's own ctx may act for another account.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(_ context.Context, tx bun.Tx) error {
		for _, ref := range p.input.Notes {
			done := Tagged{Note: ref}
			// A savepoint per note: one failing leaves the others, and the transaction usable.
			if err := tx.RunInTx(p.ctx, nil, func(_ context.Context, sp bun.Tx) error {
				return p.tagNote(sp, &done)
			}); err != nil {
				done.Error = err.Error()
			}
			p.cache.result = append(p.cache.result, done)
		}
		return nil
	}))
}

// tagNote makes the #tags of a note its own plus add, minus remove, through hashtag.Sync as patchnote does for tags.
func (p *run) tagNote(tx bun.Tx, done *Tagged) error {
	resolve := resolveitem.Process(p.ctx, tx, domain.NOTE, []string{done.Note})
	resolve.Run()
	if resolve.HasErr() {
		return resolve.GetErr()
	}
	read := readitem.Process(p.ctx, tx, domain.NOTE, resolve.GetResult()[0]).SkipChildren()
	read.Run()
	if read.HasErr() {
		return read.GetErr()
	}
	note := read.GetResult()
	before := hashtag.Parse(note.Content)
	done.AlreadyPresent, done.Added = diff(before, p.input.Add)
	done.Removed, done.NotPresent = diff(before, p.input.Remove)

	tags := slices.DeleteFunc(hashtag.Parse(note.Content), func(tag string) bool {
		return slices.ContainsFunc(p.input.Remove, func(r string) bool { return strings.EqualFold(r, tag) })
	})
	next := hashtag.Sync(note.Content, append(tags, p.input.Add...))
	for _, tag := range p.input.Add {
		if !slices.ContainsFunc(hashtag.Parse(next), func(t string) bool { return strings.EqualFold(t, tag) }) {
			return fmt.Errorf("%w: %q", ErrUnread, tag)
		}
	}
	if next != note.Content {
		// Restore keeps the text replaced even within a writing session: nothing else holds it.
		patch := patchnote.Process(p.ctx, tx, patchnote.Input{ID: note.Item.ID, Content: &next, Restore: true})
		patch.Run()
		if patch.HasErr() {
			return patch.GetErr()
		}
		note = patch.GetResult()
	}
	done.ID, done.Title, done.Path, done.Tags = note.Item.ID, string(note.Item.Title), note.Path, note.Tags
	return nil
}
