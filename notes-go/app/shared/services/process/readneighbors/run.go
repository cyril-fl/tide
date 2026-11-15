package readneighbors

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cyril-fl/tide/notes-go/app/shared/services/secret"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/searchnotes"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/wikilink"
	"github.com/uptrace/bun"
)

// Neighbor is a note sharing at least one tag with the center, with the shared tags.
type Neighbor struct {
	ID     string      `json:"id"`
	Title  secret.Text `json:"title" swaggertype:"string"`
	Path   string      `json:"path"`
	Tags   []string    `bun:",array" json:"tags"` // read as ids, shown as values
	Shared int         `json:"shared"`
}

// Linked is a note at the other end of a [[link]]; a backlink has the line of its content holding the link.
type Linked struct {
	ID      string      `json:"id"`
	Title   secret.Text `json:"title" swaggertype:"string"`
	Path    string      `json:"path"`
	Snippet string      `json:"snippet,omitempty" binding:"optional"`
	Content secret.Text `json:"-"`
}

// Result is the center note (path, content, tags), its neighbors (most shared tags first, at most the
// neighbors_limit of the account, all of them counted in NeighborsTotal), the notes its [[links]] name, and the notes
// naming it, the note it was forked from and its own forks, the one merged last.
type Result struct {
	Note           readitem.Result `json:"note"`
	Neighbors      []Neighbor      `json:"neighbors"`
	NeighborsTotal int             `json:"neighbors_total"`
	Links          []Linked        `json:"links"`
	Backlinks      []Linked        `json:"backlinks"`
	Original       []Linked        `json:"original"` // a fork: the live note it merges back into, else empty
	Forks          []Linked        `json:"forks"`
	// Mentions are the notes citing the title as a whole word without a [[link]] to the note, the latest edited first.
	Mentions []Linked `json:"mentions"`
	// Merged is the fork whose last merge into the note can still be undone (the note unchanged since), live or in the trash the merge put it in.
	Merged string `json:"merged,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	id  string
}

type cache struct {
	result Result
	tree   domain.Tree // read once for the neighbors and the links
}

func Process(ctx context.Context, db bun.IDB, id string) *run {
	return &run{
		ctx: ctx,
		db:  db,
		id:  id,
	}
}

func (p *run) Run() {
	p.loadNote()
	p.loadTree()
	p.loadNeighbors()
	p.loadLinks()
	p.loadForks()
	p.loadMentions()
}

// Getters & Setters

func (p *run) GetResult() Result {
	return p.cache.result
}

// Methods

// loadNote reuses readitem: same errors (ErrNoValue, ErrNotFound) for a missing or trashed note.
func (p *run) loadNote() {
	process := readitem.Process(p.ctx, p.db, domain.NOTE, p.id)
	process.Run()
	p.cache.result.Note = process.GetResult()
	p.SetErr(process.GetErr())
}

// loadTree reads the live items; the center's path already came from readitem.
func (p *run) loadTree() {
	if p.HasErr() {
		return
	}

	tree, err := domain.LoadTree(p.ctx, p.db)
	p.cache.tree = tree
	p.SetErr(err)
}

// loadNeighbors joins the tag links with themselves: same tag, other note. Ids only: the tree keeps the live notes
// and names them, and only the closest ones are kept, a note of a wide tag has 12,000 of them.
func (p *run) loadNeighbors() {
	if p.HasErr() {
		return
	}

	userID, id := domain.UserID(p.ctx), p.cache.result.Note.Item.ID
	var limit int
	if err := p.db.NewSelect().Model((*domain.User)(nil)).Column("neighbors_limit").Where("id = ?", userID).Scan(p.ctx, &limit); err != nil {
		p.SetErr(err)
		return
	}
	var rows []Neighbor
	p.SetErr(p.db.NewRaw(`
		SELECT i.id, array_agg(center.tag_id) AS tags, count(*) AS shared
		FROM notes_on_tags AS center
		JOIN notes_on_tags AS other ON other.tag_id = center.tag_id AND other.note_id <> center.note_id
		JOIN items AS i ON i.id = other.note_id
		WHERE center.note_id = ? AND i.user_id = ? AND i.deleted_at IS NULL
		GROUP BY i.id`, id, userID).
		Scan(p.ctx, &rows))
	if p.HasErr() {
		return
	}
	live := domain.Live(p.cache.tree, rows, func(n Neighbor) string { return n.ID }, func(a, b Neighbor) int { return b.Shared - a.Shared })
	p.cache.result.NeighborsTotal = len(live)
	p.cache.result.Neighbors = domain.Page(live, limit, 0)

	// The tags of the center, opened once each, not once per neighbor.
	var tags []domain.Tag
	p.SetErr(p.db.NewRaw(`
		SELECT t.id, t.value
		FROM tags AS t
		JOIN notes_on_tags AS nt ON nt.tag_id = t.id
		WHERE nt.note_id = ? AND t.user_id = ?`, id, userID).
		Scan(p.ctx, &tags))
	values := make(map[string]string, len(tags))
	for _, tag := range tags {
		values[tag.ID] = string(tag.Value)
	}
	for i := range p.cache.result.Neighbors {
		neighbor := &p.cache.result.Neighbors[i]
		neighbor.Title, neighbor.Path = secret.Text(p.cache.tree.Titles[neighbor.ID]), p.cache.tree.Paths[neighbor.ID]
		for t, tagID := range neighbor.Tags {
			neighbor.Tags[t] = values[tagID]
		}
		domain.SortFold(neighbor.Tags, func(v string) string { return v }, nil)
	}
}

// loadLinks reads both ends of the [[links]]; the tree keeps the live notes. Empty, not null, in JSON.
func (p *run) loadLinks() {
	if p.HasErr() {
		return
	}

	id := p.cache.result.Note.Item.ID
	for _, side := range []struct {
		from, to, content string
		dest              *[]Linked
	}{{"source_id", "target_id", "", &p.cache.result.Links}, {"target_id", "source_id", ", mn.content", &p.cache.result.Backlinks}} {
		var rows []Linked
		err := p.db.NewRaw(`
			SELECT i.id, i.title`+side.content+`
			FROM notes_on_links AS nl
			JOIN items AS i ON i.id = nl.`+side.to+`
			JOIN metadata_notes AS mn ON mn.item_id = i.id
			WHERE nl.`+side.from+` = ? AND i.user_id = ? AND i.deleted_at IS NULL`, id, domain.UserID(p.ctx)).
			Scan(p.ctx, &rows)
		if err != nil {
			p.SetErr(err)
			return
		}
		*side.dest = domain.Live(p.cache.tree, rows, func(l Linked) string { return l.ID }, nil)
		for i := range *side.dest {
			(*side.dest)[i].Path = p.cache.tree.Paths[(*side.dest)[i].ID]
		}
	}
	for i := range p.cache.result.Backlinks {
		p.cache.result.Backlinks[i].Snippet = snippet(string(p.cache.result.Backlinks[i].Content), id)
	}
}

// snippet is the first line of content linking to the note id, its links written back as [[Label]], cut around that one.
func snippet(content, id string) string {
	for _, line := range strings.Split(content, "\n") {
		at := strings.Index(strings.ToLower(line), "](note:"+strings.ToLower(id))
		if at < 0 {
			continue
		}
		start := max(0, strings.LastIndex(line[:at], "["))
		before := wikilink.Unresolve(line[:start])
		return searchnotes.Cut(before+wikilink.Unresolve(line[start:]), len(before))
	}
	return ""
}

// loadForks reads the forks made from the note; its original is in the tree already. Empty, not null, in JSON.
func (p *run) loadForks() {
	if p.HasErr() {
		return
	}

	var rows []Linked
	p.SetErr(p.db.NewRaw(`
		SELECT i.id, i.title
		FROM metadata_notes AS mn
		JOIN items AS i ON i.id = mn.item_id
		WHERE mn.fork_of = ? AND i.user_id = ? AND i.deleted_at IS NULL`, p.cache.result.Note.Item.ID, domain.UserID(p.ctx)).
		Scan(p.ctx, &rows))
	if p.HasErr() {
		return
	}
	p.cache.result.Forks = domain.Live(p.cache.tree, rows, func(l Linked) string { return l.ID }, nil)
	for i := range p.cache.result.Forks {
		p.cache.result.Forks[i].Path = p.cache.tree.Paths[p.cache.result.Forks[i].ID]
	}

	var merged []string
	p.SetErr(p.db.NewRaw(`
		SELECT mn.item_id
		FROM metadata_notes AS mn
		JOIN items AS i ON i.id = mn.item_id
		JOIN items AS o ON o.id = mn.fork_of
		WHERE mn.fork_of = ? AND i.user_id = ? AND mn.fork_merged_at = coalesce(o.updated_at, o.created_at)
			AND (i.deleted_at IS NULL OR mn.fork_merged_trash)
		LIMIT 1`, p.cache.result.Note.Item.ID, domain.UserID(p.ctx)).
		Scan(p.ctx, &merged))
	if len(merged) > 0 {
		p.cache.result.Merged = merged[0]
	}

	p.cache.result.Original = []Linked{}
	if id := p.cache.result.Note.ForkOf; id != "" {
		if path, ok := p.cache.tree.Paths[id]; ok {
			p.cache.result.Original = append(p.cache.result.Original, Linked{ID: id, Title: secret.Text(p.cache.tree.Titles[id]), Path: path})
		}
	}
}

// mentionsLimit is how many unlinked mentions are shown: the notes are opened until that many are found, at most
// mentionsOpened of them.
const (
	mentionsLimit  = 20
	mentionsOpened = 500
)

// loadMentions opens the notes the index gives for the title, latest edited first, and keeps those citing it as a
// whole word, but the note and its backlinks. None for a title under 3 letters. Empty, not null, in JSON.
// TODO: only the mentionsOpened latest edited candidates are opened, as the grams do not keep the order of the words
// (18,000 candidates opened in 600 ms for "Achat achat conférence soir" on the perf account); a positional index if older
// mentions are missed.
func (p *run) loadMentions() {
	if p.HasErr() {
		return
	}

	p.cache.result.Mentions = []Linked{}
	note := p.cache.result.Note.Item
	title := strings.TrimSpace(string(note.Title))
	if utf8.RuneCountInString(title) < 3 {
		return
	}
	query, args, _, err := searchnotes.Candidates(p.ctx, p.db, title)
	if err != nil {
		p.SetErr(err)
		return
	}
	var rows []Linked
	p.SetErr(p.db.NewRaw(`
		SELECT i.id
		FROM items AS i
		JOIN metadata_notes AS mn ON mn.item_id = i.id
		WHERE i.user_id = ? AND i.kind = ? AND i.deleted_at IS NULL AND i.id <> ?`+query+`
		ORDER BY coalesce(i.updated_at, i.created_at) DESC
		LIMIT ?`,
		append(append([]any{domain.UserID(p.ctx), domain.NOTE, note.ID}, args...), mentionsOpened)...).
		Scan(p.ctx, &rows))
	if p.HasErr() {
		return
	}
	linked := map[string]bool{}
	for _, l := range p.cache.result.Backlinks {
		linked[l.ID] = true
	}
	var ids []string
	for _, l := range domain.Live(p.cache.tree, rows, func(l Linked) string { return l.ID }, nil) {
		if !linked[l.ID] {
			ids = append(ids, l.ID)
		}
	}

	// Opened by growing batches, as searchnotes.Matches: a title in every note fills the page with the first one.
	for at, size := 0, 50; at < len(ids) && len(p.cache.result.Mentions) < mentionsLimit; at, size = at+size, size*4 {
		opened := ids[at:min(at+size, len(ids))]
		var contents []Linked
		if err := p.db.NewRaw(`
			SELECT mn.item_id AS id, mn.content
			FROM metadata_notes AS mn
			JOIN items AS i ON i.id = mn.item_id
			WHERE i.user_id = ? AND mn.item_id IN (?)`, domain.UserID(p.ctx), bun.In(opened)).
			Scan(p.ctx, &contents); err != nil {
			p.SetErr(err)
			return
		}
		byID := make(map[string]string, len(contents))
		for _, c := range contents {
			byID[c.ID] = string(c.Content)
		}
		for _, id := range opened {
			if s, ok := mention(byID[id], title); ok && len(p.cache.result.Mentions) < mentionsLimit {
				p.cache.result.Mentions = append(p.cache.result.Mentions, Linked{ID: id, Title: secret.Text(p.cache.tree.Titles[id]), Path: p.cache.tree.Paths[id], Snippet: s})
			}
		}
	}
}

// mention finds title in content as a whole word whatever its case, and gives its line cut around it: a letter on
// either side is another word (unicode.IsLetter, as \b ignores accents: "Go" is not in "Google" nor "Gö").
func mention(content, title string) (string, bool) {
	lower := strings.ToLower(title)
	for _, line := range strings.Split(content, "\n") {
		low := strings.ToLower(line)
		for from := 0; ; {
			i := strings.Index(low[from:], lower)
			if i < 0 {
				break
			}
			at, end := from+i, from+i+len(lower)
			before, _ := utf8.DecodeLastRuneInString(low[:at])
			after, _ := utf8.DecodeRuneInString(low[end:])
			if !unicode.IsLetter(before) && !unicode.IsLetter(after) {
				if len(low) != len(line) {
					line = low // a letter whose lower case has another length: the positions are those of low
				}
				head := wikilink.Unresolve(line[:at])
				return searchnotes.Cut(head+wikilink.Unresolve(line[at:]), len(head)), true
			}
			from = at + 1
		}
	}
	return "", false
}
