package replacenotes

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/cyril-fl/tide/core/services/process/searchnotes"
	"github.com/cyril-fl/tide/core/services/secret"
	"github.com/uptrace/bun"
)

var ErrNoValue = errors.New("no text to replace")

// hidden matches the targets the editor does not show: a resolved link's `](note:<id>)` and a file's URL.
var hidden = regexp.MustCompile(`\]\(note:[0-9a-fA-F-]{36}(?:#[^()\n]*)?\)|` + domain.FilePattern.String())

// maxMatches caps the occurrences a dry run lists; the counts stay exact.
const maxMatches = 500

// excerpt is the most of a line kept on each side of an occurrence, in characters.
const excerpt = 60

// Input is a plain text, no regex, found with its case. DryRun only counts and lists the occurrences, for the
// confirmation. Notes and Occurrence narrow it to what the list shows: a note, or one occurrence of it.
type Input struct {
	Find       string   `json:"find"`
	Replace    string   `json:"replace"`
	DryRun     bool     `json:"dry_run,omitempty" binding:"optional"`
	Folder     string   `json:"folder,omitempty" binding:"optional"`     // a folder id: only its notes, its subfolders included
	Notes      []string `json:"notes,omitempty" binding:"optional"`      // note ids: only these notes
	Occurrence int      `json:"occurrence,omitempty" binding:"optional"` // with Notes: only that occurrence of each, from 1
}

// Result counts the notes holding the text and its occurrences in them; the locked notes are left as they are.
// A dry run lists the occurrences too, the first maxMatches of them.
type Result struct {
	Notes       int     `json:"notes"`
	Occurrences int     `json:"occurrences"`
	Locked      int     `json:"locked"`
	Matches     []Match `json:"matches,omitempty" binding:"optional"`
}

// Match is a note holding the text, with its occurrences in order: their index is the Occurrence of Input.
type Match struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Occurrences []Line `json:"occurrences"`
}

// Line is an occurrence: its line in the note, from 1, and the text of the line around it.
type Line struct {
	Line   int    `json:"line"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx   context.Context
	db    bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	input Input
	paths map[string]string // the notes' paths, for the matches
}

type cache struct {
	result Result
}

// row is a note that may hold the text.
type row struct {
	ID       string
	Content  secret.Text
	Readonly bool
}

// Process replaces a text in the content of every live note of the account, through patchnote in one transaction:
// the locked notes are skipped and counted, and each note changed keeps a version of its text before.
// TODO: every note goes through patchnote within the 30 s of a request, fine for a few thousand changed; a job if more.
func Process(ctx context.Context, db bun.IDB, input Input) *run {
	return &run{
		ctx:   ctx,
		db:    db,
		input: input,
	}
}

func (p *run) Run() {
	p.makeInput()
	p.replace()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods

func (p *run) makeInput() {
	if strings.TrimSpace(p.input.Find) == "" {
		p.SetErr(ErrNoValue)
	}
	if p.input.Occurrence < 0 {
		p.input.Occurrence = 0
	}
}

// replace reads and writes in the same transaction, so a note saved meanwhile is not overwritten with its old text.
func (p *run) replace() {
	if p.HasErr() {
		return
	}

	// p.ctx, not the one RunInTx hands: a bun.Tx's own ctx may act for another account.
	p.SetErr(p.db.RunInTx(p.ctx, nil, func(_ context.Context, tx bun.Tx) error {
		rows, err := p.loadNotes(tx)
		if err != nil {
			return err
		}
		listed := 0
		for _, r := range rows {
			content := string(r.Content)
			at := occurrences(content, p.input.Find)
			if p.input.Occurrence > 0 {
				if p.input.Occurrence > len(at) {
					continue
				}
				at = at[p.input.Occurrence-1 : p.input.Occurrence]
			}
			if len(at) == 0 {
				continue // one the index gave without the text
			}
			if r.Readonly {
				p.cache.result.Locked++
				continue
			}
			p.cache.result.Notes++
			p.cache.result.Occurrences += len(at)
			if p.input.DryRun {
				if listed < maxMatches {
					at = at[:min(len(at), maxMatches-listed)]
					listed += len(at)
					p.cache.result.Matches = append(p.cache.result.Matches,
						Match{ID: r.ID, Path: p.paths[r.ID], Occurrences: lines(content, p.input.Find, at)})
				}
				continue
			}
			content = replaceAt(content, p.input.Find, p.input.Replace, at)
			// Restore keeps the text replaced even within a writing session: nothing else holds it.
			patch := patchnote.Process(p.ctx, tx, patchnote.Input{ID: r.ID, Content: &content, Restore: true})
			patch.Run()
			if patch.HasErr() {
				return patch.GetErr()
			}
		}
		return nil
	}))
}

// occurrences gives the offsets of the text outside the hidden targets, in order and not overlapping.
func occurrences(content, find string) []int {
	var at []int
	last := 0
	scan := func(end int) {
		for i := last; ; {
			n := strings.Index(content[i:end], find)
			if n < 0 {
				break
			}
			at = append(at, i+n)
			i += n + len(find)
		}
	}
	for _, m := range hidden.FindAllStringIndex(content, -1) {
		scan(m[0])
		last = m[1]
	}
	scan(len(content))
	return at
}

// replaceAll replaces the text outside the hidden targets, kept as they are.
func replaceAll(content, find, replace string) string {
	return replaceAt(content, find, replace, occurrences(content, find))
}

// replaceAt replaces the occurrences at the offsets given, in order.
func replaceAt(content, find, replace string, at []int) string {
	var b strings.Builder
	last := 0
	for _, i := range at {
		b.WriteString(content[last:i])
		b.WriteString(replace)
		last = i + len(find)
	}
	b.WriteString(content[last:])
	return b.String()
}

// lines gives the line of each occurrence and the text around it, cut to excerpt characters on each side.
func lines(content, find string, at []int) []Line {
	out := make([]Line, 0, len(at))
	for _, i := range at {
		start := strings.LastIndexByte(content[:i], '\n') + 1
		end := len(content)
		if n := strings.IndexByte(content[i+len(find):], '\n'); n >= 0 {
			end = i + len(find) + n
		}
		before, after := []rune(content[start:i]), []rune(content[i+len(find):end])
		if len(before) > excerpt {
			before = slices.Concat([]rune("…"), before[len(before)-excerpt:])
		}
		after = after[:min(len(after), excerpt)]
		out = append(out, Line{
			Line:   strings.Count(content[:i], "\n") + 1,
			Before: strings.TrimLeft(string(before), " \t"),
			After:  string(after),
		})
	}
	return out
}

// loadNotes reads the live notes holding every gram of the text, a few more that only share them.
func (p *run) loadNotes(tx bun.Tx) ([]row, error) {
	query, args, _, err := searchnotes.Candidates(p.ctx, tx, p.input.Find)
	if err != nil {
		return nil, err
	}
	scope, scopeArgs := "", []any{}
	if p.input.Folder != "" {
		scope = ` AND i.id IN (WITH RECURSIVE sub AS (
			SELECT id FROM items WHERE id = ? AND user_id = ?
			UNION SELECT c.id FROM items AS c JOIN sub ON c.parent_id = sub.id WHERE c.user_id = ?
		) SELECT id FROM sub)`
		scopeArgs = []any{p.input.Folder, domain.UserID(p.ctx), domain.UserID(p.ctx)}
	}
	if len(p.input.Notes) > 0 {
		scope += ` AND i.id IN (?)`
		scopeArgs = append(scopeArgs, bun.In(p.input.Notes))
	}
	var rows []row
	err = tx.NewRaw(`
		SELECT i.id, mn.content, mn.readonly
		FROM items AS i
		JOIN metadata_notes AS mn ON mn.item_id = i.id
		WHERE i.user_id = ? AND i.kind = ? AND i.deleted_at IS NULL`+scope+query,
		append(append([]any{domain.UserID(p.ctx), domain.NOTE}, scopeArgs...), args...)...).
		Scan(p.ctx, &rows)
	if err != nil {
		return nil, err
	}
	// A note in a trashed folder is not deleted itself: the tree leaves it out.
	tree, err := domain.LoadTree(p.ctx, tx)
	if err != nil {
		return nil, err
	}
	p.paths = tree.Paths
	return domain.Live(tree, rows, func(r row) string { return r.ID }, func(a, b row) int { return 0 }), nil
}
