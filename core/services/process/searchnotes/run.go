package searchnotes

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/noteindex"
	"github.com/cyril-fl/tide/core/services/notetitle"
	"github.com/cyril-fl/tide/core/services/secret"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// snippetWidth is how many characters of content a match shows around the text found.
const snippetWidth = 60

var ErrNoValue = errors.New("no text nor filter to search")

// Match is a note containing the text, with the content line where it was found.
type Match struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	Snippet  string `json:"snippet"`
	Readonly bool   `json:"readonly,omitempty" binding:"optional"`
	// The dates of the note, for a tile as a folder shows it.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitzero" binding:"optional"`
	// With the empty_sections filter, the heading lines of the sections left empty.
	EmptySections []string `json:"empty_sections,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	text   string
	limit  int // 0: every match
	offset int
	filter Filter
}

// Filter keeps the notes of a search, with or without a text: what an agent sorting the account reads first.
type Filter struct {
	Scope
	MaxLength     int       // content (title line included) of at most that many characters; 0: any length
	Untagged      bool      // notes with no tag
	UpdatedBefore time.Time // notes last changed (or created, never changed) before it, the oldest first without a text; zero: any date
	NotText       string    // notes whose title and content do not hold it, whatever its case; empty: none left out
	EmptySections bool      // notes with a heading followed by nothing up to one of its level or higher, or the end
}

// Scope keeps the notes by their tags and folders, every list left empty keeping them all: Idée AND (Cuisine OR
// Boisson) is TagsAll [Idée] and TagsAny [Cuisine, Boisson].
type Scope struct {
	TagsAll        []string // tag ids the notes all carry
	TagsAny        []string // tag ids the notes carry at least one of
	TagsNone       []string // tag ids the notes carry none of
	Folders        []string // folder ids holding the notes, in their subfolders too
	ExcludeFolders bool     // the notes out of Folders instead
}

// Where gives the conditions on i (items) keeping the notes of the scope, to add to a WHERE, and their arguments.
func (s Scope) Where(ctx context.Context) (string, []any) {
	var query string
	var args []any
	if len(s.TagsAll) > 0 {
		query += ` AND i.id IN (
			SELECT note_id FROM notes_on_tags WHERE tag_id IN (?) GROUP BY note_id HAVING COUNT(DISTINCT tag_id) = ?)`
		args = append(args, bun.In(s.TagsAll), len(slices.Compact(slices.Sorted(slices.Values(s.TagsAll)))))
	}
	if len(s.TagsAny) > 0 {
		query += ` AND i.id IN (SELECT note_id FROM notes_on_tags WHERE tag_id IN (?))`
		args = append(args, bun.In(s.TagsAny))
	}
	if len(s.TagsNone) > 0 {
		query += ` AND i.id NOT IN (SELECT note_id FROM notes_on_tags WHERE tag_id IN (?))`
		args = append(args, bun.In(s.TagsNone))
	}
	if len(s.Folders) > 0 {
		tree := `(WITH RECURSIVE tree AS (
			SELECT id FROM items WHERE id IN (?) AND user_id = ?
			UNION ALL SELECT child.id FROM items AS child JOIN tree ON child.parent_id = tree.id
		) SELECT id FROM tree)`
		if s.ExcludeFolders {
			query += ` AND i.id NOT IN ` + tree
		} else {
			query += ` AND i.id IN ` + tree
		}
		args = append(args, bun.In(s.Folders), domain.UserID(ctx))
	}
	return query, args
}

// empty is true when the filter keeps every note.
func (f Filter) empty() bool {
	return f.MaxLength == 0 && !f.Untagged && !f.EmptySections && f.UpdatedBefore.IsZero() && f.NotText == "" &&
		len(f.TagsAll)+len(f.TagsAny)+len(f.TagsNone)+len(f.Folders) == 0
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
	ID        string
	Content   secret.Text
	Readonly  bool
	CreatedAt time.Time
	UpdatedAt time.Time
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

// Filter keeps only the notes filter accepts; with one, the text may be empty.
func (p *run) Filter(filter Filter) *run {
	p.filter = filter
	return p
}

func (p *run) Run() {
	p.makeText()
	p.loadCandidates()
	p.loadMatches()
}

// Getters & Setters

// GetResult returns the matching notes: the text in their title first, then in a heading, then the most backlinks,
// the latest edited, by path.
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
	if p.cache.text == "" && p.filter.empty() {
		p.SetErr(ErrNoValue)
	}
}

// candidate is a note that may hold the text, with what ranks it: no note is opened.
type candidate struct {
	ID        string
	InHeading bool
	UpdatedAt time.Time // created_at for a note never edited
	InTitle   bool      `bun:"-"`
	Backlinks int       `bun:"-"`
}

// loadCandidates reads the live notes holding every gram of the text, ranked without opening any: the text in their
// title (from the tree), then in a heading, then the most backlinks from live notes, the latest edited, by path.
func (p *run) loadCandidates() {
	if p.HasErr() {
		return
	}

	query, args, grams, err := Candidates(p.ctx, p.db, p.cache.text)
	if err != nil {
		p.SetErr(err)
		return
	}
	if p.filter.Untagged {
		query += " AND NOT EXISTS (SELECT 1 FROM notes_on_tags AS nt WHERE nt.note_id = i.id)"
	}
	if !p.filter.UpdatedBefore.IsZero() {
		query += " AND coalesce(i.updated_at, i.created_at) < ?"
		args = append(args, p.filter.UpdatedBefore)
	}
	scope, scopeArgs := p.filter.Where(p.ctx)
	query += scope
	args = append(args, scopeArgs...)
	var rows []candidate
	p.SetErr(p.db.NewRaw(`
		SELECT i.id, coalesce(i.updated_at, i.created_at) AS updated_at, coalesce(mn.headings @> ?::smallint[], false) AS in_heading
		FROM items AS i
		JOIN metadata_notes AS mn ON mn.item_id = i.id
		WHERE i.user_id = ? AND i.kind = ? AND i.deleted_at IS NULL`+query,
		append([]any{pgdialect.Array(grams), domain.UserID(p.ctx), domain.NOTE}, args...)...).
		Scan(p.ctx, &rows))
	if p.HasErr() {
		return
	}
	// Every link of the account in one read: a subquery per note costs 130 ms at 50,000 notes.
	var links []domain.NoteLink
	p.SetErr(p.db.NewRaw(`
		SELECT nl.source_id, nl.target_id
		FROM notes_on_links AS nl
		JOIN items AS i ON i.id = nl.source_id
		WHERE i.user_id = ?`, domain.UserID(p.ctx)).
		Scan(p.ctx, &links))
	if p.HasErr() {
		return
	}

	p.cache.tree, err = domain.LoadTree(p.ctx, p.db)
	p.SetErr(err)
	backlinks := map[string]int{}
	for _, link := range links {
		if _, live := p.cache.tree.Paths[link.SourceID]; live {
			backlinks[link.TargetID]++
		}
	}
	text := strings.ToLower(p.cache.text)
	for i := range rows { // set once: map lookups in the comparison cost 70 ms at 50,000 notes
		rows[i].InTitle = strings.Contains(strings.ToLower(p.cache.tree.Titles[rows[i].ID]), text)
		rows[i].Backlinks = backlinks[rows[i].ID]
	}
	oldestFirst := text == "" && !p.filter.UpdatedBefore.IsZero() // looking for stale notes: the first page the oldest
	live := domain.Live(p.cache.tree, rows, func(c candidate) string { return c.ID }, func(a, b candidate) int {
		if oldestFirst {
			return a.UpdatedAt.Compare(b.UpdatedAt)
		}
		return cmp.Or(
			boolOrder(b.InTitle)-boolOrder(a.InTitle),
			boolOrder(b.InHeading)-boolOrder(a.InHeading),
			b.Backlinks-a.Backlinks,
			b.UpdatedAt.Compare(a.UpdatedAt),
		)
	})
	p.cache.ids = make([]string, len(live))
	for i, c := range live {
		p.cache.ids[i] = c.ID
	}
}

func (p *run) loadMatches() {
	if p.HasErr() {
		return
	}

	var err error
	p.cache.result, p.cache.total, err = Matches(p.ctx, p.db, p.cache.tree, p.cache.ids, p.cache.text, p.filter, p.limit, p.offset)
	p.SetErr(err)
}

// Helpers

// batch is how many notes Matches opens first, maxBatch at once: each read opens 4 times more, a page takes one
// read and a search without a limit not hundreds of them.
const (
	batch    = 200
	maxBatch = 12800
)

// Matches opens the notes of ids, in their order, and keeps those holding text in their title or content whatever
// its case and passing the content filters of filter (NotText, MaxLength, EmptySections; the others are the caller's):
// limit of them from offset, every one when limit is 0. No text nor content filter: every note, none opened for its
// content.
// The index gives a few notes that do not hold the text, so they are opened until the page is full, not all of them:
// the total counts the matches met and the notes left unopened (none with a content filter: all are opened, so the
// total is exact), and a page costs the ones before it.
// TODO: the last pages of a search with tens of thousands of matches open them all (1 s at 50,000 notes), as every
// page did; a cursor instead of an offset if they are ever read.
func Matches(ctx context.Context, db bun.IDB, tree domain.Tree, ids []string, text string, filter Filter, limit, offset int) ([]Match, int, error) {
	notText, maxLength := filter.NotText, filter.MaxLength
	lower, notLower := strings.ToLower(text), strings.ToLower(notText)
	// Every note is opened when its content filters it: the unopened ones cannot be assumed to pass.
	exact := maxLength > 0 || notText != "" || filter.EmptySections
	inTitle := func(id string) bool { return strings.Contains(strings.ToLower(tree.Titles[id]), lower) }
	columns := "mn.item_id AS id, mn.readonly, i.created_at, i.updated_at"
	if text != "" || exact {
		columns += ", mn.content"
	}
	if limit <= 0 {
		offset = 0
	}
	limit, offset = min(limit, len(ids)), min(offset, len(ids)) // huge numbers: offset+limit must not overflow

	// The notes before the page whose title holds the text are counted unopened, unless their length is checked.
	at := 0
	for !exact && at < offset && at < len(ids) && inTitle(ids[at]) {
		at++
	}
	found := at
	var matches []Match
	full := func() bool { return !exact && limit > 0 && found >= offset+limit }
	for size := batch; at < len(ids) && !full(); size = min(size*4, maxBatch) {
		opened := ids[at:min(at+size, len(ids))]
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
			if full() {
				break
			}
			at++
			r, ok := byID[id]
			if !ok || !inTitle(id) && !strings.Contains(strings.ToLower(string(r.Content)), lower) {
				continue // trashed since, or one the index gave without the text
			}
			if maxLength > 0 && utf8.RuneCountInString(string(r.Content)) > maxLength {
				continue
			}
			// The content starts with the title line, the title in the tree is checked too should it not.
			if notText != "" && (strings.Contains(strings.ToLower(string(r.Content)), notLower) || strings.Contains(strings.ToLower(tree.Titles[id]), notLower)) {
				continue
			}
			var empty []string
			if filter.EmptySections {
				if empty = notetitle.EmptySections(string(r.Content)); len(empty) == 0 {
					continue
				}
			}
			if found >= offset && (limit <= 0 || found < offset+limit) {
				match := Match{ID: id, Title: tree.Titles[id], Path: tree.Paths[id], Readonly: r.Readonly, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, EmptySections: empty}
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
// keeps the notes holding every gram of text, and those grams for mn.headings: a few more share them, the caller
// checks the text once opened. None for a single letter: every note, and no grams.
// TODO: a 2-letter text ("go") opens about a note in 30 more than hold it (16-bit blinds), fine at 50,000 notes.
func Candidates(ctx context.Context, db bun.IDB, text string) (string, []any, []int16, error) {
	if err := noteindex.Fill(ctx, db); err != nil {
		return "", nil, nil, err
	}
	grams, err := noteindex.Query(domain.UserID(ctx), text)
	if err != nil || grams == nil {
		return "", nil, nil, err
	}
	return " AND mn.grams @> ?::smallint[]", []any{pgdialect.Array(grams)}, grams, nil
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
