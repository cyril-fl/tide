package domain

import (
	"context"
	"time"

	"github.com/cyril-fl/tide/notes-go/app/shared/services/secret"
	"github.com/uptrace/bun"
)

type Kind string

const (
	FOLDER Kind = "folder"
	NOTE   Kind = "note"
)

type Item struct {
	bun.BaseModel `bun:"table:items,alias:i" json:"-"`

	ID         string      `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID     string      `bun:",type:uuid,notnull,nullzero" json:"-"`
	ParentID   *string     `bun:",type:uuid,nullzero" json:"parent_id" binding:"optional"`
	Parent     *Item       `bun:"rel:belongs-to,join:parent_id=id" json:"-"`
	CreatedAt  time.Time   `bun:",notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time   `bun:",nullzero" json:"updated_at,omitzero" binding:"optional"`
	DeletedAt  time.Time   `bun:",soft_delete,nullzero" json:"-"`
	Kind       Kind        `bun:",notnull" json:"kind"`
	Title      secret.Text `bun:",type:text,nullzero" json:"title" swaggertype:"string"`     // NULL = untitled note (--no-title)
	TitleHash  string      `bun:",nullzero" json:"-"`                                        // its Blind: unique folder names, [[links]] by title
	Readonly   bool        `bun:",scanonly" json:"readonly,omitempty" binding:"optional"`    // a locked note, read by listitems from metadata_notes
	HasFolders bool        `bun:",scanonly" json:"has_folders,omitempty" binding:"optional"` // a folder with subfolders, read by listitems
}

// SealTitle gives a title sealed, and its blind for title_hash: both "" for an untitled note, written NULL.
func SealTitle(ctx context.Context, title string) (secret.Text, string, error) {
	if title == "" {
		return "", "", nil
	}
	sealed, err := secret.Seal(UserID(ctx), title)
	if err != nil {
		return "", "", err
	}
	hash, err := secret.Blind(UserID(ctx), title)
	return sealed, hash, err
}

type Metadata struct {
	ItemID string `bun:",pk,type:uuid"`
	Item   *Item  `bun:"rel:belongs-to,join:item_id=id"`
}

type MetadataNote struct {
	bun.BaseModel `bun:"table:metadata_notes,alias:mn"`
	Metadata      `bun:",inline"`

	Content  secret.Text `bun:",type:text"`
	Readonly bool        `bun:",notnull"`
	Pinned   bool        `bun:",notnull"`
	Grams    []int16     `bun:",array,type:smallint[]"` // noteindex: blinds of its grams, NULL until noteindex.Fill
	Waiting  []string    `bun:",array,type:text[]"`     // noteindex: blinds of the titles its [[links]] wait for
	Linked   bool        // noteindex: its text has a link or a file listbrokenlinks checks, NULL until noteindex.Fill
	// A fork: the note it was made from, which mergefork brings its changes back into; NULL for any other note.
	ForkOf string `bun:",type:uuid,nullzero"`
	// The fork's text, sealed, at the fork then at each merge: the base of the next three-way merge.
	ForkBase secret.Text `bun:",type:text,nullzero"`
	// The last merge, what unmergefork puts back: the base before it, the original's text before it, sealed, the
	// original's version it saved (updated_at or created_at) and whether it trashed the fork. NULL before a merge and after its undo.
	ForkUndoBase    secret.Text `bun:",type:text,nullzero"`
	ForkUndoContent secret.Text `bun:",type:text,nullzero"`
	ForkMergedAt    time.Time   `bun:",nullzero"`
	ForkMergedTrash bool

	Headings     []int16  `bun:",array,type:smallint[]"` // noteindex: blinds of the grams of its headings
	HeadingNames []string `bun:",array,type:text[]"`     // noteindex: blind of each heading's noteindex.HeadingName
	Tags         []Tag    `bun:"m2m:notes_on_tags,join:Note=Tag"`
}

type MetadataFolder struct {
	bun.BaseModel `bun:"table:metadata_folders,alias:mf"`
	Metadata      `bun:",inline"`

	Pinned bool `bun:",notnull,default:false"` // a favorite
}

// NoteVersion is a text a note had before an edit replaced it, kept by patchnote (#61). Deleted with its note.
type NoteVersion struct {
	bun.BaseModel `bun:"table:note_versions,alias:nv" json:"-"`

	ID        string      `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	ItemID    string      `bun:",type:uuid,notnull" json:"-"`
	Content   secret.Text `bun:",type:text,notnull" json:"content,omitempty" swaggertype:"string" binding:"optional"` // left out of a list
	FileIDs   []string    `bun:"file_ids,array,type:uuid[]" json:"-"`                                                 // the images Content cites, in clear: purgefiles keeps them
	Conflict  bool        `bun:",notnull,default:false" json:"conflict"`                                              // a text refused by a 409, kept so it is not lost (#78)
	CreatedAt time.Time   `bun:",notnull,default:current_timestamp" json:"created_at"`
}

// NewNoteVersion seals text for the account of ctx and notes the images it cites.
func NewNoteVersion(ctx context.Context, itemID, text string) (NoteVersion, error) {
	content, err := secret.Seal(UserID(ctx), text)
	version := NoteVersion{ItemID: itemID, Content: content}
	for _, match := range FilePattern.FindAllStringSubmatch(text, -1) {
		version.FileIDs = append(version.FileIDs, match[1])
	}
	return version, err
}
