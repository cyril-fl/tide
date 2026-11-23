// Package readsavedsearch finds a saved search by id among the account's, then replays its criteria.
package readsavedsearch

import (
	"context"
	"errors"
	"fmt"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/runsavedsearch"
	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("saved search not found")

type run struct {
	plumprimitive.Process
	cache

	ctx    context.Context
	db     bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	id     string
	limit  int // 0: every match
	offset int
}

type cache struct {
	search domain.SavedSearch
	result []runsavedsearch.Match
	total  int
}

func Process(ctx context.Context, db bun.IDB, id string) *run {
	return &run{
		ctx: ctx,
		db:  db,
		id:  id,
	}
}

// Page keeps limit matches from offset; GetTotal still counts them.
func (p *run) Page(limit, offset int) *run {
	p.limit, p.offset = limit, offset
	return p
}

func (p *run) Run() {
	p.findSearch()
	p.runSearch()
}

// Getters & Setters

func (p *run) GetResult() []runsavedsearch.Match {
	return p.cache.result
}

// GetTotal counts the matches as runsavedsearch does.
func (p *run) GetTotal() int {
	return p.cache.total
}

// Methods

// findSearch loads the account's saved searches and keeps the one asked for.
func (p *run) findSearch() {
	var searches domain.SavedSearches
	err := p.db.NewRaw("SELECT saved_searches FROM users WHERE id = ?", domain.UserID(p.ctx)).Scan(p.ctx, &searches)
	if err != nil {
		p.SetErr(err)
		return
	}

	for _, search := range searches {
		if search.ID == p.id && !search.Group { // a group holds no criteria: it would match every note
			p.cache.search = search
			return
		}
	}
	p.SetErr(fmt.Errorf("%w: %q", ErrNotFound, p.id))
}

func (p *run) runSearch() {
	if p.HasErr() {
		return
	}

	process := runsavedsearch.Process(p.ctx, p.db, p.cache.search).Page(p.limit, p.offset)
	process.Run()
	p.SetErr(process.GetErr())
	p.cache.result, p.cache.total = process.GetResult(), process.GetTotal()
}
