package replacenotes

import (
	"context"
	"errors"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/searchnotes"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

var ErrNoValue = errors.New("no text to replace")

// Input is a plain text, no regex, found with its case. DryRun only counts, for the confirmation.
type Input struct {
	Find    string `json:"find"`
	Replace string `json:"replace"`
	DryRun  bool   `json:"dry_run,omitempty" binding:"optional"`
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
			count := strings.Count(string(r.Content), p.input.Find)
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
			content := strings.ReplaceAll(string(r.Content), p.input.Find, p.input.Replace)
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

// loadNotes reads the live notes holding every gram of the text, a few more that only share them.
func (p *run) loadNotes(tx bun.Tx) ([]row, error) {
	query, args, _, err := searchnotes.Candidates(p.ctx, tx, p.input.Find)
	if err != nil {
		return nil, err
	}
	var rows []row
	err = tx.NewRaw(`
		SELECT i.id, mn.content, mn.readonly
		FROM items AS i
		JOIN metadata_notes AS mn ON mn.item_id = i.id
		WHERE i.user_id = ? AND i.kind = ? AND i.deleted_at IS NULL`+query,
		append([]any{domain.UserID(p.ctx), domain.NOTE}, args...)...).
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
