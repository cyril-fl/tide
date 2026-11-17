package resolveitem

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	plumprimitive "github.com/cyril-fl/plum/primitive"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/uptrace/bun"
)

var (
	ErrNotFound  = errors.New("no item at this path")
	ErrAmbiguous = errors.New("several items at this path, use a uuid")
)

type run struct {
	plumprimitive.Process
	cache

	ctx  context.Context
	db   bun.IDB // *bun.DB or bun.Tx: tests run inside a rolled-back transaction
	kind domain.Kind
	refs []string
}

// candidate is an item at the path asked.
type candidate struct {
	ID       string      `bun:"id"`
	Kind     domain.Kind `bun:"kind"`
	Modified time.Time   `bun:"modified"`
}

type cache struct {
	paths  map[string]string // loaded at the first path, for every path of the refs
	result []string
}

// Process turns references into ids: a path (/Projects/Go/Ideas) is looked up among the live items
// of the kind (any kind when empty), anything else is left for the caller to treat as a uuid.
func Process(ctx context.Context, db bun.IDB, kind domain.Kind, refs []string) *run {
	return &run{
		ctx:  ctx,
		db:   db,
		kind: kind,
		refs: refs,
	}
}

func (p *run) Run() {
	p.resolveRefs()
}

// Getters & Setters

// GetResult returns one id per reference, in the same order.
func (p *run) GetResult() []string {
	return p.cache.result
}

// Methods

func (p *run) resolveRefs() {
	for _, ref := range p.refs {
		if p.HasErr() {
			return
		}

		if !strings.HasPrefix(ref, "/") {
			p.cache.result = append(p.cache.result, ref)
			continue
		}
		p.resolvePath(ref)
	}
}

// resolvePath fails on several matches: titles are not unique, and acting on the wrong note is worse than an error.
func (p *run) resolvePath(path string) {
	if p.cache.paths == nil {
		paths, err := domain.Paths(p.ctx, p.db)
		if err != nil {
			p.SetErr(err)
			return
		}
		p.cache.paths = paths
	}
	var ids []string
	var err error
	for id, at := range p.cache.paths {
		if at == path {
			ids = append(ids, id)
		}
	}
	var all, found []candidate
	if len(ids) > 0 {
		err = p.db.NewRaw("SELECT id, kind, coalesce(updated_at, created_at) AS modified FROM items WHERE id IN (?) ORDER BY id", bun.In(ids)).Scan(p.ctx, &all)
	}
	if err != nil {
		p.SetErr(err)
		return
	}
	for _, c := range all {
		if p.kind == "" || c.Kind == p.kind {
			found = append(found, c)
		}
	}

	switch {
	case len(all) > 0 && len(found) == 0:
		// Said apart from a wrong path, so the caller turns to the right command instead of searching for it.
		p.SetErr(fmt.Errorf("%w: %q is a %s, not a %s", ErrNotFound, path, all[0].Kind, p.kind))
	case len(found) == 0:
		p.SetErr(fmt.Errorf("%w: %q", ErrNotFound, path))
	case len(found) == 1:
		p.cache.result = append(p.cache.result, found[0].ID)
	default:
		// The kind and the date of each, for the caller to tell them apart without reading them all.
		described := make([]string, 0, len(found))
		for _, c := range found {
			described = append(described, fmt.Sprintf("%s (%s changed %s)", c.ID, c.Kind, c.Modified.UTC().Format(time.RFC3339)))
		}
		p.SetErr(fmt.Errorf("%w: %q is %s", ErrAmbiguous, path, strings.Join(described, ", ")))
	}
}
