package readneighbors

import (
	"context"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/searchnotes"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/wikilink"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

// Neighbor is a note sharing at least one tag with the center, with the shared tags.
type Neighbor struct {
	ID     string      `json:"id"`
	Title  secret.Text `json:"title" swaggertype:"string"`
	Path   string      `json:"path"`
	Tags   []string    `bun:",array" json:"tags"`
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

// Result is the center note (path, content, tags), its neighbors (most shared tags first),
// the notes its [[links]] name, and the notes naming it.
type Result struct {
	Note      readitem.Result `json:"note"`
	Neighbors []Neighbor      `json:"neighbors"`
	Links     []Linked        `json:"links"`
	Backlinks []Linked        `json:"backlinks"`
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

// loadNeighbors joins the tag links with themselves: same tag, other note; the tree keeps the live ones.
// The tags come sealed, opened and ordered here.
func (p *run) loadNeighbors() {
	if p.HasErr() {
		return
	}

	var rows []Neighbor
	p.SetErr(p.db.NewRaw(`
		SELECT i.id, i.title, array_agg(t.value) AS tags, count(*) AS shared
		FROM notes_on_tags AS center
		JOIN notes_on_tags AS other ON other.tag_id = center.tag_id AND other.note_id <> center.note_id
		JOIN tags AS t ON t.id = center.tag_id
		JOIN items AS i ON i.id = other.note_id
		WHERE center.note_id = ? AND i.user_id = ? AND i.deleted_at IS NULL
		GROUP BY i.id, i.title`, p.cache.result.Note.Item.ID, domain.UserID(p.ctx)).
		Scan(p.ctx, &rows))
	if p.HasErr() {
		return
	}

	p.cache.result.Neighbors = domain.Live(p.cache.tree, rows, func(n Neighbor) string { return n.ID }, func(a, b Neighbor) int { return b.Shared - a.Shared })
	for i := range p.cache.result.Neighbors {
		neighbor := &p.cache.result.Neighbors[i]
		neighbor.Path = p.cache.tree.Paths[neighbor.ID]
		if !p.HasErr() {
			var err error
			neighbor.Tags, err = domain.OpenTags(neighbor.Tags)
			p.SetErr(err)
		}
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
