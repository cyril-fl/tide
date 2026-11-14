package domain

import (
	"context"
	"sync"

	"github.com/uptrace/bun"
)

// Versioned keeps one value per account while its users.items_version stays the same: built again after a write that
// adds, removes, renames, moves, trashes or restores an item, whoever made it. In memory only, never written.
type Versioned[T any] struct {
	accounts sync.Map // by account id
}

type versioned[T any] struct {
	version int64
	value   T
}

// Load gives the value of the account of ctx, built when its items changed since the last call.
func (c *Versioned[T]) Load(ctx context.Context, db bun.IDB, build func() (T, error)) (T, error) {
	userID := UserID(ctx)
	// The version before the items: a write committed between the two only makes the next call rebuild.
	var version int64
	if err := db.NewSelect().Model((*User)(nil)).Column("items_version").Where("id = ?", userID).Scan(ctx, &version); err != nil {
		var none T
		return none, err
	}
	if cached, ok := c.accounts.Load(userID); ok && cached.(versioned[T]).version == version {
		return cached.(versioned[T]).value, nil
	}

	value, err := build()
	if err != nil {
		return value, err
	}
	c.accounts.Store(userID, versioned[T]{version, value})
	return value, nil
}

// Tree is every live item of an account, by id: its path from its root (/Projects/Go/Ideas) and its title in clear.
// A trashed folder hides its whole subtree, so an id missing from it is not live. Shared, do not modify it.
type Tree struct {
	Paths  map[string]string
	Titles map[string]string
}

// trees keeps the Tree of each account. Titles in clear: in memory only.
var trees Versioned[Tree]

// LoadTree gives the Tree of the account of ctx. Titles are sealed: paths are made here, not in SQL.
// Cached (one query, not 75 ms of decryption at 50,000 items): reading it opens nothing.
func LoadTree(ctx context.Context, db bun.IDB) (Tree, error) {
	return trees.Load(ctx, db, func() (Tree, error) { return readTree(ctx, db) })
}

// Paths gives the paths of LoadTree.
func Paths(ctx context.Context, db bun.IDB) (map[string]string, error) {
	tree, err := LoadTree(ctx, db)
	return tree.Paths, err
}

// Titles gives the titles of LoadTree.
func Titles(ctx context.Context, db bun.IDB) (map[string]string, error) {
	tree, err := LoadTree(ctx, db)
	return tree.Titles, err
}

func readTree(ctx context.Context, db bun.IDB) (Tree, error) {
	var items []Item
	err := db.NewSelect().Model(&items).Column("id", "parent_id", "title").Where("user_id = ?", UserID(ctx)).Scan(ctx)
	if err != nil {
		return Tree{}, err
	}

	children := map[string][]Item{}
	for _, item := range items {
		parent := ""
		if item.ParentID != nil {
			parent = *item.ParentID
		}
		children[parent] = append(children[parent], item)
	}
	tree := Tree{Paths: map[string]string{}, Titles: map[string]string{}}
	var walk func(parent, prefix string)
	walk = func(parent, prefix string) {
		for _, item := range children[parent] {
			title := string(item.Title)
			tree.Titles[item.ID] = title
			if title == "" {
				title = "untitled"
			}
			tree.Paths[item.ID] = prefix + "/" + title
			walk(item.ID, tree.Paths[item.ID])
		}
	}
	walk("", "")
	return tree, nil
}

// Live keeps the rows whose item (id gives it) is live, sorted by first then by path, as SortFold does: what every
// list of notes shows. The rows only need their ids: the titles and paths come from the tree, nothing is opened.
func Live[T any](tree Tree, rows []T, id func(T) string, first func(a, b T) int) []T {
	live := make([]T, 0, len(rows))
	for _, row := range rows {
		if _, ok := tree.Paths[id(row)]; ok {
			live = append(live, row)
		}
	}
	SortFold(live, func(row T) string { return tree.Paths[id(row)] }, first)
	return live
}

// Page keeps limit rows from offset; every row when limit is 0.
func Page[T any](rows []T, limit, offset int) []T {
	if limit <= 0 {
		return rows
	}
	start := min(max(offset, 0), len(rows))
	return rows[start : start+min(limit, len(rows)-start)] // limit may be huge: no start+limit
}
