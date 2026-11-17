package readnotes

import (
	"context"
	"errors"
	"fmt"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/cyril-fl/tide/core/services/process/resolveitem"
	"github.com/uptrace/bun"
)

// MaxNotes is how many notes one call reads: an agent's batch, not the account.
const MaxNotes = 50

var ErrInvalid = errors.New("invalid argument")

// Note is a note as readitem gives it, with the whole content's length and truncated when max_chars cut it.
type Note struct {
	readitem.Result
	Length    *int `json:"length,omitempty" binding:"optional"` // the whole content's characters, given with max_chars
	Truncated bool `json:"truncated,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx      context.Context
	db       bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	refs     []string
	maxChars int
}

type cache struct {
	ids    []string
	result []Note
}

// Process reads notes by uuid or path, in the order of refs; one missing fails it, named in the error. maxChars > 0
// keeps only the start of each content.
func Process(ctx context.Context, db bun.IDB, refs []string, maxChars int) *run {
	return &run{
		ctx:      ctx,
		db:       db,
		refs:     refs,
		maxChars: maxChars,
	}
}

func (p *run) Run() {
	p.checkInput()
	p.resolveRefs()
	p.readNotes()
}

// Getters & Setters

func (p *run) GetResult() []Note {
	return p.cache.result
}

// Methods

func (p *run) checkInput() {
	switch {
	case len(p.refs) == 0 || len(p.refs) > MaxNotes:
		p.SetErr(fmt.Errorf("%w: notes: %d items received, between 1 and %d wanted", ErrInvalid, len(p.refs), MaxNotes))
	case p.maxChars < 0:
		p.SetErr(fmt.Errorf("%w: max_chars: %d received, 0 or more wanted", ErrInvalid, p.maxChars))
	}
}

func (p *run) resolveRefs() {
	if p.HasErr() {
		return
	}

	process := resolveitem.Process(p.ctx, p.db, domain.NOTE, p.refs)
	process.Run()
	p.cache.ids = process.GetResult()
	p.SetErr(process.GetErr())
}

// readNotes loads the account's paths once for the batch, not once per note.
func (p *run) readNotes() {
	if p.HasErr() {
		return
	}
	paths, err := domain.Paths(p.ctx, p.db)
	if err != nil {
		p.SetErr(err)
		return
	}
	for i, id := range p.cache.ids {
		if p.HasErr() {
			return
		}

		process := readitem.Process(p.ctx, p.db, domain.NOTE, id).WithPaths(paths)
		process.Run()
		if process.HasErr() {
			p.SetErr(fmt.Errorf("%s: %w", p.refs[i], process.GetErr()))
			return
		}
		p.cache.result = append(p.cache.result, preview(process.GetResult(), p.maxChars))
	}
}

// Helpers

func preview(result readitem.Result, maxChars int) Note {
	read := Note{Result: result}
	read.Tags = append([]string{}, read.Tags...)
	if maxChars <= 0 {
		return read
	}
	content := []rune(read.Content)
	length := len(content)
	read.Length, read.Truncated = &length, length > maxChars
	if read.Truncated {
		read.Content = string(content[:maxChars])
	}
	return read
}
