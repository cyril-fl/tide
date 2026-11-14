// Package noteindex keeps on each note what finds it without opening it (#145): the blinds of the grams of its title
// and content for searchnotes, the blinds of the titles its [[links]] wait for for linknotes, whether it has a link
// or a file and the blinds of its headings for listbrokenlinks, the blinds of the grams of its headings (#169). A trigger of createschema empties them when the content changes, whoever writes it;
// Fill makes them again before they are read.
package noteindex

import (
	"context"
	"strings"

	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/notetitle"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/secret"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/wikilink"
	"github.com/uptrace/bun"
)

// chunk is how many notes Fill opens and writes at once: the first Fill of a big account indexes them all.
const chunk = 1000

// Grams gives the pieces of 2 and 3 letters of text lowered: a text holding another holds all its grams.
func Grams(text string) []string {
	runes := []rune(strings.ToLower(text))
	var grams []string
	for i := range runes {
		for _, n := range []int{2, 3} {
			if i+n <= len(runes) {
				grams = append(grams, string(runes[i:i+n]))
			}
		}
	}
	return grams
}

// Query gives the blinds a note holding text has, whatever its case: its trigrams, its bigram when it has 2 letters.
// nil for a single letter: every note is a candidate.
func Query(userID, text string) ([]int16, error) {
	runes := []rune(strings.ToLower(text))
	if len(runes) < 2 {
		return nil, nil
	}
	var grams []string
	for _, gram := range Grams(text) {
		if len([]rune(gram)) == min(3, len(runes)) {
			grams = append(grams, gram)
		}
	}
	return secret.BlindGrams(userID, grams)
}

// Waiting gives the blinds of the titles the [[links]] of content wait for: the target before its #heading, and the
// last name of a [[/Folder/Title]]. The titles of the notes it could resolve against are blinded the same way.
func Waiting(userID, content string) ([]string, error) {
	blinds := []string{}
	for _, target := range wikilink.Parse(content) {
		name, _, _ := strings.Cut(target, "#")
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		blind, err := secret.Blind(userID, name)
		if err != nil {
			return nil, err
		}
		blinds = append(blinds, blind)
	}
	return blinds, nil
}

// Linked tells whether content has what listbrokenlinks checks: a [[link]], a link to a note, the address of a file.
func Linked(content string) bool {
	return len(wikilink.Parse(content)) > 0 || len(wikilink.IDs(content)) > 0 || domain.FilePattern.MatchString(content)
}

// HeadingName gives heading as a [[Note#Heading]] names it: no parentheses, trimmed. Blind ignores the case.
func HeadingName(heading string) string {
	return strings.TrimSpace(strings.NewReplacer("(", "", ")", "").Replace(heading))
}

// Headings gives the blinds of the grams of the headings of content, and the blind of each heading's HeadingName.
func Headings(userID, content string) ([]int16, []string, error) {
	var grams []string
	names := []string{}
	for _, heading := range notetitle.Headings(content) {
		grams = append(grams, Grams(heading)...)
		name, err := secret.Blind(userID, HeadingName(heading))
		if err != nil {
			return nil, nil, err
		}
		names = append(names, name)
	}
	blinds, err := secret.BlindGrams(userID, grams)
	return blinds, names, err
}

// Fill indexes the notes of the account of ctx, trashed too, whose content changed since their last Fill (grams NULL).
// A note written again meanwhile keeps NULL: the content compared is the one indexed.
func Fill(ctx context.Context, db bun.IDB) error {
	userID := domain.UserID(ctx)
	for {
		var rows []struct {
			ID      string      `bun:"id"`
			Title   secret.Text `bun:"title"`
			Content secret.Text `bun:"content"`
			Sealed  string      `bun:"sealed"`
		}
		err := db.NewRaw(`
			SELECT mn.item_id AS id, i.title, mn.content, mn.content AS sealed
			FROM metadata_notes AS mn JOIN items AS i ON i.id = mn.item_id
			WHERE mn.grams IS NULL AND i.user_id = ? LIMIT ?`, userID, chunk).Scan(ctx, &rows)
		if err != nil || len(rows) == 0 {
			return err
		}

		type index struct {
			ItemID  string   `bun:"item_id,type:uuid"`
			Sealed  string   `bun:"sealed,type:text"`
			Grams   []int16  `bun:"grams,array,type:smallint[]"`
			Waiting []string `bun:"waiting,array,type:text[]"`
			Linked  bool     `bun:"linked"`

			Headings     []int16  `bun:"headings,array,type:smallint[]"`
			HeadingNames []string `bun:"heading_names,array,type:text[]"`
		}
		indexes := make([]index, len(rows))
		for i, row := range rows {
			grams, err := secret.BlindGrams(userID, append(Grams(string(row.Title)), Grams(string(row.Content))...))
			if err != nil {
				return err
			}
			waiting, err := Waiting(userID, string(row.Content))
			if err != nil {
				return err
			}
			headings, names, err := Headings(userID, string(row.Content))
			if err != nil {
				return err
			}
			indexes[i] = index{ItemID: row.ID, Sealed: row.Sealed, Grams: grams, Waiting: waiting, Linked: Linked(string(row.Content)),
				Headings: headings, HeadingNames: names}
		}
		_, err = db.NewUpdate().
			With("indexed", db.NewValues(&indexes)).
			Model((*domain.MetadataNote)(nil)).
			TableExpr("indexed").
			Set("grams = indexed.grams, waiting = indexed.waiting, linked = indexed.linked").
			Set("headings = indexed.headings, heading_names = indexed.heading_names").
			Where("mn.item_id = indexed.item_id AND mn.content = indexed.sealed").
			Exec(ctx)
		if err != nil || len(rows) < chunk {
			return err
		}
	}
}

// FillAll runs Fill for every account with notes left to index: notes-server serve runs it at start and in its loop,
// so a base just upgraded is indexed before its first search, and the notes of an idle account do not slow the Fill
// of the others.
func FillAll(ctx context.Context, db bun.IDB) error {
	var ids []string
	err := db.NewRaw(`
		SELECT DISTINCT i.user_id FROM metadata_notes AS mn JOIN items AS i ON i.id = mn.item_id
		WHERE mn.grams IS NULL`).Scan(ctx, &ids)
	for _, id := range ids {
		if err == nil {
			err = secret.Load(ctx, db, id)
		}
		if err == nil {
			err = Fill(domain.WithUser(ctx, id), db)
		}
	}
	return err
}
