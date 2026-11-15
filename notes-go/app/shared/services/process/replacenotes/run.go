package replacenotes

import (
	"context"
	"errors"
	"regexp"
	"strings"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/searchnotes"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/secret"
	"github.com/uptrace/bun"
)

var ErrNoValue = errors.New("no text to replace")

// hidden matches the targets the editor does not show: a resolved link's `](note:<id>)` and a file's URL.
var hidden = regexp.MustCompile(`\]\(note:[0-9a-fA-F-]{36}(?:#[^()\n]*)?\)|` + domain.FilePattern.String())

// Input is a plain text, no regex, found with its case. DryRun only counts, for the confirmation.
type Input struct {
	Find    string `json:"find"`
	Replace string `json:"replace"`
	DryRun  bool   `json:"dry_run,omitempty" binding:"optional"`
	Folder  string `json:"folder,omitempty" binding:"optional"` // a folder id: only its notes, its subfolders included
}

// Result counts the notes holding the text and its occurrences in them; the locked notes are left as they are.
type Result struct {
	Notes       int `json:"notes"`
	Occurrences int `json:"occurrences"`
	Locked      int `json:"locked"`
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
		for _, r := range rows {
			count := count(string(r.Content), p.input.Find)
			if count == 0 {
				continue // one the index gave without the text
			}
			if r.Readonly {
				p.cache.result.Locked++
				continue
			}
			p.cache.result.Notes++
			p.cache.result.Occurrences += count
			if p.input.DryRun {
				continue
			}
			content := replaceAll(string(r.Content), p.input.Find, p.input.Replace)
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

// count counts the text outside the hidden targets.
func count(content, find string) int {
	n := 0
	for _, text := range hidden.Split(content, -1) {
		n += strings.Count(text, find)
	}
	return n
}

// replaceAll replaces the text outside the hidden targets, kept as they are.
func replaceAll(content, find, replace string) string {
	var b strings.Builder
	last := 0
	for _, m := range hidden.FindAllStringIndex(content, -1) {
		b.WriteString(strings.ReplaceAll(content[last:m[0]], find, replace))
		b.WriteString(content[m[0]:m[1]])
		last = m[1]
	}
	b.WriteString(strings.ReplaceAll(content[last:], find, replace))
	return b.String()
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
	return domain.Live(tree, rows, func(r row) string { return r.ID }, func(a, b row) int { return 0 }), nil
}
