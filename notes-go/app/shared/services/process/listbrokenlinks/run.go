package listbrokenlinks

import (
	"context"
	"slices"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/noteindex"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/notetitle"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readgraph"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/wikilink"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

// The kinds of broken link.
const (
	UNRESOLVED = "unresolved" // [[target]] that no other note has as title or path, a heading the note lacks
	AMBIGUOUS  = "ambiguous"  // [[target]] that several other notes have
	MISSING    = "missing"    // a link to a note trashed or purged since
	FILE       = "file"       // an image or attachment /api/files/<id> the account does not keep: imported from another, purged
)

// Link is a broken link of a live note: its target as written, the label of a link to a missing note, or the address of a missing file.
type Link struct {
	NoteID    string `json:"note_id"`
	NoteTitle string `json:"note_title"`
	NotePath  string `json:"note_path"`
	Target    string `json:"target"`
	Kind      string `json:"kind" enums:"unresolved,ambiguous,missing,file"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx context.Context
	db  bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
}

type cache struct {
	notes    []readgraph.Note
	contents map[string]string
	headings map[string][]string // the heading_names of the notes a link points to a heading of
	files    map[string]bool
	result   []Link
}

// Process lists the links of the live notes that point to no note, or wait for a title several notes have, and the files they cite that the account lacks.
func Process(ctx context.Context, db bun.IDB) *run {
	return &run{
		ctx: ctx,
		db:  db,
	}
}

func (p *run) Run() {
	p.loadContents()
	p.loadNotes()
	p.loadTargets()
	p.loadFiles()
	p.check()
}

// Getters & Setters

func (p *run) GetResult() []Link {
	return p.cache.result
}

// Methods

// loadContents opens only the notes with a link or a file to check, found by their index: opening every live note
// costs 0.5 s at 50,000 notes.
func (p *run) loadContents() {
	p.SetErr(noteindex.Fill(p.ctx, p.db))
	p.cache.contents = map[string]string{}
	p.open("mn.linked")
}

// loadNotes reads the live notes, their titles and paths, sorted by path: the graph's nodes. None when no note has
// anything to check.
func (p *run) loadNotes() {
	if p.HasErr() || len(p.cache.contents) == 0 {
		return
	}

	graph := readgraph.Process(p.ctx, p.db, false)
	graph.Run()
	if graph.HasErr() {
		p.SetErr(graph.GetErr())
		return
	}
	p.cache.notes = graph.GetResult().Notes
}

// loadTargets reads the blinds of the headings of the notes a link points to a heading of, from their index: they are
// not opened (#169).
func (p *run) loadTargets() {
	p.cache.headings = map[string][]string{}
	var ids []string
	for _, content := range p.cache.contents {
		for _, link := range wikilink.HeadingLinks(content) {
			ids = append(ids, link.ID)
		}
	}
	if p.HasErr() || len(ids) == 0 {
		return
	}

	var rows []struct {
		ID       string   `bun:"id"`
		Headings []string `bun:"heading_names,array"`
	}
	p.SetErr(p.db.NewRaw(`
		SELECT mn.item_id AS id, mn.heading_names
		FROM metadata_notes AS mn
		JOIN items AS i ON i.id = mn.item_id
		WHERE i.user_id = ? AND i.deleted_at IS NULL AND mn.item_id::text IN (?)`, domain.UserID(p.ctx), bun.In(ids)).
		Scan(p.ctx, &rows))
	for _, row := range rows {
		p.cache.headings[row.ID] = row.Headings
	}
}

// loadFiles reads the ids of the files the account keeps.
func (p *run) loadFiles() {
	if p.HasErr() {
		return
	}

	var ids []string
	p.SetErr(p.db.NewSelect().Model((*domain.File)(nil)).Column("id").
		Where("user_id = ?", domain.UserID(p.ctx)).Scan(p.ctx, &ids))
	p.cache.files = make(map[string]bool, len(ids))
	for _, id := range ids {
		p.cache.files[id] = true
	}
}

// check counts, for each [[target]], the other notes with that title (any case) or that path, as linknotes.Resolve
// finds them, and looks for the ids of resolved links among the live notes.
func (p *run) check() {
	if p.HasErr() {
		return
	}

	named := map[string][]string{}
	live := map[string]bool{}
	for _, note := range p.cache.notes {
		named[strings.ToLower(string(note.Title))] = append(named[strings.ToLower(string(note.Title))], note.ID)
		named[note.Path] = append(named[note.Path], note.ID)
		live[note.ID] = true
	}

	for _, note := range p.cache.notes {
		content := p.cache.contents[note.ID]
		add := func(target, kind string) {
			p.cache.result = append(p.cache.result, Link{note.ID, string(note.Title), note.Path, target, kind})
		}
		for _, target := range wikilink.Parse(content) {
			name, heading, _ := strings.Cut(target, "#")
			if name = strings.TrimSpace(name); name == "" {
				// [[#Heading]]: a heading of the note itself.
				if !hasHeading(content, heading) {
					add(target, UNRESOLVED)
				}
				continue
			}
			if !strings.HasPrefix(name, "/") {
				name = strings.ToLower(name)
			}
			others := 0
			for _, id := range named[name] {
				if id != note.ID {
					others++
				}
			}
			if others == 0 {
				add(target, UNRESOLVED)
			} else if others > 1 {
				add(target, AMBIGUOUS)
			}
		}
		for _, link := range wikilink.HeadingLinks(content) {
			if !live[link.ID] {
				continue
			}
			blind, err := secret.Blind(domain.UserID(p.ctx), noteindex.HeadingName(link.Heading))
			if err != nil {
				p.SetErr(err)
				return
			}
			if !slices.Contains(p.cache.headings[link.ID], blind) {
				add(link.Label, UNRESOLVED)
			}
		}
		labels := wikilink.Labels(content)
		for _, id := range wikilink.IDs(content) {
			if !live[strings.ToLower(id)] {
				add(labels[strings.ToLower(id)], MISSING)
			}
		}
		for _, match := range domain.FilePattern.FindAllStringSubmatch(content, -1) {
			if !p.cache.files[match[1]] {
				add(match[0], FILE)
			}
		}
	}
}

// Helpers

// open adds to the contents the live notes of the account that match where.
func (p *run) open(where string, args ...any) {
	if p.HasErr() {
		return
	}

	var rows []struct {
		ID      string      `bun:"id"`
		Content secret.Text `bun:"content"`
	}
	p.SetErr(p.db.NewRaw(`
		SELECT mn.item_id AS id, mn.content
		FROM metadata_notes AS mn
		JOIN items AS i ON i.id = mn.item_id
		WHERE i.user_id = ? AND i.deleted_at IS NULL AND `+where, append([]any{domain.UserID(p.ctx)}, args...)...).
		Scan(p.ctx, &rows))
	for _, row := range rows {
		p.cache.contents[row.ID] = string(row.Content)
	}
}

// hasHeading tells whether content has heading, as the app compares them: any case, no parentheses.
func hasHeading(content, heading string) bool {
	return slices.ContainsFunc(notetitle.Headings(content), func(h string) bool {
		return strings.EqualFold(noteindex.HeadingName(h), noteindex.HeadingName(heading))
	})
}
