// Package runsavedsearch replays a domain.SavedSearch's criteria: never a stored result, always run again.
package runsavedsearch

import (
	"context"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/searchnotes"
	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/uptrace/bun"
)

// Match is a note found by the saved search, with the content line where the text criterion was found.
type Match struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	Snippet  string `json:"snippet,omitempty"`
	Readonly bool   `json:"readonly,omitempty" binding:"optional"`
}

type run struct {
	plumprimitive.Process
	cache

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	search domain.SavedSearch
	limit  int // 0: every match
	offset int
}

type cache struct {
	tree   domain.Tree
	ids    []string // the live notes the SQL criteria keep, by path
	result []Match
	total  int
}

func Process(ctx context.Context, db bun.IDB, search domain.SavedSearch) *run {
	return &run{
		ctx:    ctx,
		db:     db,
		search: search,
	}
}

// Page keeps limit matches from offset, in the same order; GetTotal still counts them.
func (p *run) Page(limit, offset int) *run {
	p.limit, p.offset = limit, offset
	return p
}

func (p *run) Run() {
	p.queryCandidates()
	p.filterText()
}

// Getters & Setters

// GetResult returns the matching notes, sorted by path.
func (p *run) GetResult() []Match {
	return p.cache.result
}

// GetTotal counts the matches as searchnotes.Matches does: exact without a text criterion.
func (p *run) GetTotal() int {
	return p.cache.total
}

// Methods

// queryCandidates runs every SQL-filterable criterion (tags, dates, folders), and the grams of the text criterion if
// any, for the ids of the live notes: titles and paths come from the tree, no note is opened.
func (p *run) queryCandidates() {
	query := `
		SELECT i.id
		FROM items AS i
		JOIN metadata_notes AS mn ON mn.item_id = i.id
		WHERE i.user_id = ? AND i.kind = ? AND i.deleted_at IS NULL`
	args := []any{domain.UserID(p.ctx), domain.NOTE}

	if len(p.search.TagIDs) > 0 {
		if p.search.TagMode == "all" {
			query += ` AND i.id IN (
				SELECT note_id FROM notes_on_tags WHERE tag_id IN (?) GROUP BY note_id HAVING COUNT(DISTINCT tag_id) = ?)`
			args = append(args, bun.In(p.search.TagIDs), len(p.search.TagIDs))
		} else {
			query += ` AND i.id IN (SELECT note_id FROM notes_on_tags WHERE tag_id IN (?))`
			args = append(args, bun.In(p.search.TagIDs))
		}
	}
	if p.search.CreatedFrom != nil {
		query += ` AND i.created_at >= ?`
		args = append(args, *p.search.CreatedFrom)
	}
	if p.search.CreatedTo != nil {
		query += ` AND i.created_at <= ?`
		args = append(args, *p.search.CreatedTo)
	}
	if p.search.EditedFrom != nil {
		query += ` AND i.updated_at >= ?`
		args = append(args, *p.search.EditedFrom)
	}
	if p.search.EditedTo != nil {
		query += ` AND i.updated_at <= ?`
		args = append(args, *p.search.EditedTo)
	}
	if len(p.search.FolderIDs) > 0 {
		tree := `(WITH RECURSIVE tree AS (
			SELECT id FROM items WHERE id IN (?) AND user_id = ?
			UNION ALL SELECT child.id FROM items AS child JOIN tree ON child.parent_id = tree.id
		) SELECT id FROM tree)`
		if p.search.FolderMode == "exclude" {
			query += ` AND i.id NOT IN ` + tree
		} else {
			query += ` AND i.id IN ` + tree
		}
		args = append(args, bun.In(p.search.FolderIDs), domain.UserID(p.ctx))
	}

	if text := strings.TrimSpace(p.search.Text); text != "" {
		grams, gramArgs, _, err := searchnotes.Candidates(p.ctx, p.db, text)
		if err != nil {
			p.SetErr(err)
			return
		}
		query += grams
		args = append(args, gramArgs...)
	}

	var ids []string
	p.SetErr(p.db.NewRaw(query, args...).Scan(p.ctx, &ids))
	if p.HasErr() {
		return
	}

	var err error
	p.cache.tree, err = domain.LoadTree(p.ctx, p.db)
	p.SetErr(err)
	p.cache.ids = domain.Live(p.cache.tree, ids, func(id string) string { return id }, nil)
}

// filterText opens the page and checks the text itself; without a text criterion the SQL criteria already did the filtering.
func (p *run) filterText() {
	if p.HasErr() {
		return
	}

	matches, total, err := searchnotes.Matches(p.ctx, p.db, p.cache.tree, p.cache.ids, strings.TrimSpace(p.search.Text), p.limit, p.offset)
	p.SetErr(err)
	p.cache.total = total
	for _, match := range matches {
		p.cache.result = append(p.cache.result, Match(match))
	}
}
