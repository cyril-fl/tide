package searchnotes

import (
	"context"
	"errors"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/noteindex"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// snippetWidth is how many characters of content a match shows around the text found.
const snippetWidth = 60

var ErrNoValue = errors.New("no text to search")

// Match is a note containing the text, with the content line where it was found.
type Match struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	Snippet  string `json:"snippet"`
	Readonly bool   `json:"readonly,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	text   string
	limit  int // 0: every match
	offset int
}

type cache struct {
	text   string
	tree   domain.Tree
	ids    []string // the live notes that may hold the text, in the order of the result
	result []Match
	total  int
}

// row is a note opened for the page: its whole content, cut down to a snippet.
type row struct {
	ID       string
	Content  secret.Text
	Readonly bool
}

func Process(ctx context.Context, db bun.IDB, text string) *run {
	return &run{
		ctx:  ctx,
		db:   db,
		text: text,
	}
}

// Page keeps limit matches from offset, in the same order; GetTotal still counts them.
func (p *run) Page(limit, offset int) *run {
	p.limit, p.offset = limit, offset
	return p
}

func (p *run) Run() {
	p.makeText()
	p.loadCandidates()
	p.loadMatches()
}

// Getters & Setters

// GetResult returns the matching notes: title matches first, then by path.
func (p *run) GetResult() []Match {
	return p.cache.result
}

// GetTotal counts the matches, and the notes after the page that may match: exact once the last page is read.
func (p *run) GetTotal() int {
	return p.cache.total
}

// Methods

func (p *run) makeText() {
	p.cache.text = strings.TrimSpace(p.text)
	if p.cache.text == "" {
		p.SetErr(ErrNoValue)
	}
}

// loadCandidates reads the ids of the live notes holding every gram of the text, the ones with it in their title
// first, then by path: titles and paths come from the tree, no note is opened.
func (p *run) loadCandidates() {
	if p.HasErr() {
		return
	}

	query, args, err := Candidates(p.ctx, p.db, p.cache.text)
	if err != nil {
		p.SetErr(err)
		return
	}
	var ids []string
	p.SetErr(p.db.NewRaw(`
		SELECT i.id
		FROM items AS i
		JOIN metadata_notes AS mn ON mn.item_id = i.id
		WHERE i.user_id = ? AND i.kind = ? AND i.deleted_at IS NULL`+query,
		append([]any{domain.UserID(p.ctx), domain.NOTE}, args...)...).
		Scan(p.ctx, &ids))
	if p.HasErr() {
		return
	}

	p.cache.tree, err = domain.LoadTree(p.ctx, p.db)
	p.SetErr(err)
	text := strings.ToLower(p.cache.text)
	inTitle := make(map[string]bool, len(ids))
	for _, id := range ids {
		inTitle[id] = strings.Contains(strings.ToLower(p.cache.tree.Titles[id]), text)
	}
	p.cache.ids = domain.Live(p.cache.tree, ids, func(id string) string { return id }, func(a, b string) int {
		return boolOrder(inTitle[b]) - boolOrder(inTitle[a])
	})
}

func (p *run) loadMatches() {
	if p.HasErr() {
		return
	}

	var err error
	p.cache.result, p.cache.total, err = Matches(p.ctx, p.db, p.cache.tree, p.cache.ids, p.cache.text, p.limit, p.offset)
	p.SetErr(err)
}

// Helpers

// batch is how many notes Matches opens at once.
const batch = 200

// Matches opens the notes of ids, in their order, and keeps those holding text in their title or content whatever
// its case: limit of them from offset, every one when limit is 0. No text: every note, none opened for its content.
// The index gives a few notes that do not hold the text, so they are opened until the page is full, not all of them:
// the total counts the matches met and the notes left unopened, and a page costs the ones before it.
// TODO: the last pages of a search with tens of thousands of matches open them all (1 s at 50,000 notes), as every
// page did; a cursor instead of an offset if they are ever read.
func Matches(ctx context.Context, db bun.IDB, tree domain.Tree, ids []string, text string, limit, offset int) ([]Match, int, error) {
	lower := strings.ToLower(text)
	inTitle := func(id string) bool { return strings.Contains(strings.ToLower(tree.Titles[id]), lower) }
	columns := "mn.item_id AS id, mn.readonly"
	if text != "" {
		columns += ", mn.content"
	}
	if limit <= 0 {
		offset = 0
	}
	limit, offset = min(limit, len(ids)), min(offset, len(ids)) // huge numbers: offset+limit must not overflow

	// The notes before the page whose title holds the text are counted unopened.
	at := 0
	for at < offset && at < len(ids) && inTitle(ids[at]) {
		at++
	}
	found := at
	var matches []Match
	for at < len(ids) && (limit <= 0 || found < offset+limit) {
		opened := ids[at:min(at+batch, len(ids))]
		var rows []row
		err := db.NewRaw(`
			SELECT `+columns+`
			FROM metadata_notes AS mn
			JOIN items AS i ON i.id = mn.item_id
			WHERE i.user_id = ? AND mn.item_id IN (?)`, domain.UserID(ctx), bun.In(opened)).
			Scan(ctx, &rows)
		if err != nil {
			return nil, 0, err
		}
		byID := make(map[string]row, len(rows))
		for _, r := range rows {
			byID[r.ID] = r
		}
		for _, id := range opened {
			if limit > 0 && found == offset+limit {
				break
			}
			at++
			r, ok := byID[id]
			if !ok || !inTitle(id) && !strings.Contains(strings.ToLower(string(r.Content)), lower) {
				continue // trashed since, or one the index gave without the text
			}
			if found >= offset {
				match := Match{ID: id, Title: tree.Titles[id], Path: tree.Paths[id], Readonly: r.Readonly}
				if text != "" {
					match.Snippet = Snippet(string(r.Content), text)
				}
				matches = append(matches, match)
			}
			found++
		}
	}
	return matches, found + len(ids) - at, nil
}

// Candidates indexes the notes written since the last search, then gives the condition on mn (metadata_notes) that
// keeps the notes holding every gram of text: a few more share them, the caller checks the text once opened.
// None for a single letter: every note.
// TODO: a 2-letter text ("go") opens about a note in 30 more than hold it (16-bit blinds), fine at 50,000 notes.
func Candidates(ctx context.Context, db bun.IDB, text string) (string, []any, error) {
	if err := noteindex.Fill(ctx, db); err != nil {
		return "", nil, err
	}
	grams, err := noteindex.Query(domain.UserID(ctx), text)
	if err != nil || grams == nil {
		return "", nil, err
	}
	return " AND mn.grams @> ?::smallint[]", []any{pgdialect.Array(grams)}, nil
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Snippet is the first content line containing text, cut around it; the first line when only the title matched.
func Snippet(content, text string) string {
	lines := strings.Split(content, "\n")
	line, at := strings.TrimSpace(lines[0]), 0
	for _, l := range lines {
		if i := strings.Index(strings.ToLower(l), strings.ToLower(text)); i >= 0 {
			line, at = l, i
			break
		}
	}
	return Cut(line, at)
}

// Cut is a content line cut to snippetWidth around the byte at, with … where cut.
func Cut(line string, at int) string {
	runes := []rune(line)
	if len(runes) <= snippetWidth {
		return strings.TrimSpace(line)
	}
	start := max(0, len([]rune(line[:at]))-snippetWidth/3)
	end := min(len(runes), start+snippetWidth)
	cut := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		cut = "…" + cut
	}
	if end < len(runes) {
		cut += "…"
	}
	return cut
}
