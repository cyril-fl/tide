package editnotetext

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/notetitle"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/uptrace/bun"
)

var (
	ErrNoValue = errors.New("give one of title, old or section")
	ErrNoMatch = errors.New("text not found in the note")
	ErrMany    = errors.New("text found more than once in the note")
)

var (
	fence = regexp.MustCompile("^[ \\t]*(```|~~~)")
	// heading is the start of a heading line, up to its words, as the app's retitle reads it (web/app/utils/title.ts).
	heading = regexp.MustCompile(`^( {0,3}(#{1,6})[ \t]+)\S`)
)

// Input is one change of a note's text: Title renames it, Old is replaced by New, or Section has its body replaced by New.
type Input struct {
	ID    string `json:"-"`
	Title string `json:"title,omitempty" binding:"optional"` // the new title: the first heading takes it, or one is added on top
	Old   string `json:"old,omitempty" binding:"optional"`   // an exact text, with its case, to find once (or every time with all)
	New   string `json:"new,omitempty" binding:"optional"`   // what replaces old or the body of section, empty to delete it
	All   bool   `json:"all,omitempty" binding:"optional"`   // replace every occurrence of old instead of refusing more than one
	// Section is the words of a heading: what follows it, down to the next heading of its level or higher, becomes New.
	Section string `json:"section,omitempty" binding:"optional"`
}

// Result is the note as saved, and how many times old or section was replaced (1 for a rename).
type Result struct {
	Note     readitem.Result `json:"note"`
	Replaced int             `json:"replaced"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
}

type cache struct {
	result Result
}

// Process changes a part of a note through patchnote, the path of a full edit: a locked note refuses, the text
// replaced goes to the history as an edit's would, the #tags and the title follow the new text, the links the new title.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.makeInput()
	p.edit()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods

func (p *run) makeInput() {
	p.input.Title = strings.TrimSpace(p.input.Title)
	given := 0
	for _, field := range []string{p.input.Title, p.input.Old, strings.TrimSpace(p.input.Section)} {
		if field != "" {
			given++
		}
	}
	if given != 1 || strings.ContainsAny(p.input.Title, "\r\n") {
		p.SetErr(ErrNoValue)
	}
}

// edit reads and writes in one transaction, so a save in between is not overwritten with the text read.
func (p *run) edit() {
	if p.HasErr() {
		return
	}

	// p.ctx, not the one RunInTx hands: a bun.Tx's own ctx may act for another account.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(_ context.Context, tx bun.Tx) error {
		read := readitem.Process(p.ctx, tx, domain.NOTE, p.input.ID)
		read.Run()
		if read.HasErr() {
			return read.GetErr()
		}
		if read.GetResult().Readonly {
			return patchnote.ErrLocked // before the text is looked at: a locked note refuses whatever it holds
		}
		content, replaced, err := p.change(read.GetResult().Content)
		if err != nil {
			return err
		}
		patch := patchnote.Process(p.ctx, tx, patchnote.Input{ID: p.input.ID, Content: &content})
		patch.Run()
		if patch.HasErr() {
			return patch.GetErr()
		}
		p.cache.result = Result{Note: patch.GetResult(), Replaced: replaced}
		return nil
	}))
}

func (p *run) change(content string) (string, int, error) {
	switch {
	case p.input.Title != "":
		return Retitle(content, p.input.Title), 1, nil
	case p.input.Old != "":
		n := strings.Count(content, p.input.Old)
		switch {
		case n == 0:
			return "", 0, fmt.Errorf("%w: %q", ErrNoMatch, p.input.Old)
		case n > 1 && !p.input.All:
			return "", 0, fmt.Errorf("%w: %q found %d times, give more of the text around it, or all", ErrMany, p.input.Old, n)
		}
		return strings.ReplaceAll(content, p.input.Old, p.input.New), n, nil
	}
	return replaceSection(content, strings.TrimSpace(p.input.Section), p.input.New)
}

// Helpers

// Retitle gives content with title in its first heading, else a heading added on top, as the app renames a note.
// TODO: the marks and #tags of that heading are replaced too, like the app's rename; keep them if renamed headings lose tags.
func Retitle(content, title string) string {
	lines := strings.Split(content, "\n")
	for i, kind := range kinds(lines) {
		if kind > 0 {
			lines[i] = heading.FindStringSubmatch(lines[i])[1] + title
			return strings.Join(lines, "\n")
		}
	}
	if content == "" {
		return "# " + title
	}
	return "# " + title + "\n\n" + content
}

// replaceSection puts body under the one heading whose words are section, down to the next heading of its level or higher.
func replaceSection(content, section, body string) (string, int, error) {
	lines := strings.Split(content, "\n")
	levels := kinds(lines)
	start, n := -1, 0
	for i, level := range levels {
		if level > 0 && notetitle.Heading(lines[i]) == section {
			start, n = i, n+1
		}
	}
	switch {
	case n == 0:
		return "", 0, fmt.Errorf("%w: no heading %q", ErrNoMatch, section)
	case n > 1:
		return "", 0, fmt.Errorf("%w: heading %q found %d times", ErrMany, section, n)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if levels[i] > 0 && levels[i] <= levels[start] {
			end = i
			break
		}
	}
	out := append([]string{}, lines[:start+1]...)
	if body = strings.Trim(body, "\n"); body != "" {
		out = append(out, "", body)
	}
	if end < len(lines) {
		out = append(out, "")
		out = append(out, lines[end:]...)
	}
	return strings.Join(out, "\n"), 1, nil
}

// kinds gives the level of each heading line, 0 for any other line and those of fenced code.
func kinds(lines []string) []int {
	levels := make([]int, len(lines))
	code := false
	for i, line := range lines {
		if fence.MatchString(line) {
			code = !code
			continue
		}
		if match := heading.FindStringSubmatch(line); !code && match != nil {
			levels[i] = len(match[2])
		}
	}
	return levels
}
