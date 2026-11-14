package domain

import (
	"github.com/cyril-fl/notes/notes-go/app/shared/services/fold"
	"github.com/uptrace/bun"
)

// RegisterModels must run before any query on MetadataNote.Tags (bun m2m requirement).
func RegisterModels(db *bun.DB) {
	db.RegisterModel((*NoteTag)(nil))
}

// SortFold is fold.Sort, for the processes: the CLI sorts the same way without the models.
func SortFold[T any](items []T, key func(T) string, first func(a, b T) int) {
	fold.Sort(items, key, first)
}
