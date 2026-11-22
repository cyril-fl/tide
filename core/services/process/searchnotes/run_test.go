package searchnotes

import (
	"context"
	"errors"
	"fmt"
	"github.com/cyril-fl/tide/core/services/testaccount"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/editnote"
)

// No DB needed: makeText fails first and the search is guarded, so a nil db is never touched.
func TestNoValue(t *testing.T) {
	process := Process(context.Background(), nil, " ")
	process.Run()
	if !errors.Is(process.GetErr(), ErrNoValue) {
		t.Fatalf("err = %v, want %v", process.GetErr(), ErrNoValue)
	}
}

func TestSnippet(t *testing.T) {
	long := strings.Repeat("a", 80) + " needle " + strings.Repeat("b", 80)
	tests := map[string]struct{ content, text, want string }{
		"line found":   {"first\nthe NEEDLE here\nlast", "needle", "the NEEDLE here"},
		"title only":   {"  first line  \nsecond", "absent", "first line"},
		"cut around":   {long, "needle", "…" + strings.Repeat("a", 19) + " needle " + strings.Repeat("b", 33) + "…"},
		"short enough": {"x needle y", "needle", "x needle y"},
	}

	for name, test := range tests {
		if got := Snippet(test.content, test.text); got != test.want {
			t.Errorf("%s: Snippet() = %q, want %q", name, got, test.want)
		}
	}
}

func TestSearchNotes(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, tx = testaccount.New(t, ctx, tx)

	// Unique per run so real notes cannot match.
	word := fmt.Sprintf("zq%d", time.Now().UnixNano())
	ids := map[string]string{}
	for title, content := range map[string]string{
		"A in content":    "line\nhas " + strings.ToUpper(word) + " inside",
		"Z " + word:       "title match",
		"Percent":         word + "100% sure",
		"Trashed " + word: "C",
	} {
		process := createnote.Process(ctx, tx, createnote.Input{Content: "# " + title + "\n\n" + content})
		process.Run()
		if process.HasErr() {
			t.Fatalf("note %s: %v", title, process.GetErr())
		}
		ids[title] = process.GetResult().ID
	}
	trash := deleteitems.Process(ctx, tx, domain.NOTE, []string{ids["Trashed "+word]})
	trash.Run()
	if trash.HasErr() {
		t.Fatalf("trash: %v", trash.GetErr())
	}

	// Title match first, then by path; case-insensitive; the trashed note is gone.
	search := Process(ctx, tx, word)
	search.Run()
	var titles []string
	for _, match := range search.GetResult() {
		titles = append(titles, match.Title)
	}
	if search.HasErr() || strings.Join(titles, ",") != "Z "+word+",A in content,Percent" {
		t.Fatalf("titles = %v, %v", titles, search.GetErr())
	}
	if search.GetResult()[0].CreatedAt.IsZero() {
		t.Errorf("created_at is empty, want the note's date for its tile")
	}
	if got := search.GetResult()[1].Snippet; got != "has "+strings.ToUpper(word)+" inside" {
		t.Errorf("snippet = %q, want the matching line", got)
	}
	// By pages: the second match alone, the three still counted.
	page := Process(ctx, tx, word).Page(1, 1)
	page.Run()
	if page.HasErr() || len(page.GetResult()) != 1 || page.GetResult()[0].Title != "A in content" || page.GetTotal() != 3 {
		t.Errorf("page = %+v of %d, %v; want A in content of 3", page.GetResult(), page.GetTotal(), page.GetErr())
	}
	// A note the index gave without the text is opened, dropped and not counted: the page is filled by the next one.
	tree, err := domain.LoadTree(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	candidates := []string{ids["Percent"], ids["Z "+word], ids["A in content"]}
	filled, total, err := Matches(ctx, tx, tree, candidates, "inside", Filter{}, 1, 0)
	if err != nil || len(filled) != 1 || filled[0].ID != ids["A in content"] || total != 1 {
		t.Errorf("matches = %+v of %d, %v; want A in content of 1", filled, total, err)
	}
	if _, err := tx.NewRaw("UPDATE metadata_notes SET readonly = true WHERE item_id = ?", ids["A in content"]).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	search = Process(ctx, tx, word)
	search.Run()
	if search.HasErr() || !search.GetResult()[1].Readonly {
		t.Errorf("search readonly = %v, %v; want true", search.GetResult(), search.GetErr())
	}

	// % is literal: only the note containing "100%" matches.
	percent := Process(ctx, tx, word+"100%")
	percent.Run()
	if percent.HasErr() || len(percent.GetResult()) != 1 || percent.GetResult()[0].Title != "Percent" {
		t.Errorf("percent = %+v, %v; want only Percent", percent.GetResult(), percent.GetErr())
	}

	// Any piece of a word is found, not only its start.
	inside := Process(ctx, tx, word[2:])
	inside.Run()
	if inside.HasErr() || len(inside.GetResult()) != 3 {
		t.Errorf("inside = %+v, %v; want the 3 live notes", inside.GetResult(), inside.GetErr())
	}

	// An edit indexes the note again: its old text is no longer found, its new one is.
	edit := editnote.Process(ctx, tx, editnote.Input{ID: ids["Percent"], Content: "# Percent\n\nnow " + word + "Edited"})
	edit.Run()
	if edit.HasErr() {
		t.Fatalf("edit: %v", edit.GetErr())
	}
	for text, want := range map[string]int{word + "100%": 0, word + "edited": 1} {
		search := Process(ctx, tx, text)
		search.Run()
		if search.HasErr() || len(search.GetResult()) != want {
			t.Errorf("search %q after the edit = %+v, %v; want %d", text, search.GetResult(), search.GetErr(), want)
		}
	}
}

func TestSearchOrder(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, tx = testaccount.New(t, ctx, tx)

	// Unique per run so real notes cannot match; the targets of the links come before the notes linking to them.
	word := fmt.Sprintf("zq%d", time.Now().UnixNano())
	ids := map[string]string{}
	for _, note := range [][2]string{
		{"Old", word}, {"Recent", word}, {"B path", word}, {"A path", word},
		{"Cited once", word}, {"Cited twice", word}, {"Heading", "## The " + word + " part"}, {"Title " + word, "x"},
		{"Linker 1", "[[Cited twice]] [[Cited once]]"}, {"Linker 2", "[[Cited twice]]"},
		{"Trashed 1", "[[Cited once]]"}, {"Trashed 2", "[[Cited once]]"},
	} {
		process := createnote.Process(ctx, tx, createnote.Input{Content: "# " + note[0] + "\n\n" + note[1]})
		process.Run()
		if process.HasErr() {
			t.Fatalf("note %s: %v", note[0], process.GetErr())
		}
		ids[note[0]] = process.GetResult().ID
	}
	trash := deleteitems.Process(ctx, tx, domain.NOTE, []string{ids["Trashed 1"], ids["Trashed 2"]})
	trash.Run()
	if trash.HasErr() {
		t.Fatalf("trash: %v", trash.GetErr())
	}
	if _, err := tx.NewRaw("UPDATE items SET created_at = now() - interval '1 day' WHERE id = ?", ids["Old"]).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	// Title, heading, backlinks from live notes (Cited once has 1, not 3), latest edited, path. A single letter has
	// no grams: no heading criterion, Heading falls back to its path.
	for text, want := range map[string]string{
		word: "Title " + word + ",Heading,Cited twice,Cited once,A path,B path,Recent,Old",
		"z":  "Title " + word + ",Cited twice,Cited once,A path,B path,Heading,Recent,Old",
	} {
		search := Process(ctx, tx, text)
		search.Run()
		var titles []string
		for _, match := range search.GetResult() {
			titles = append(titles, match.Title)
		}
		if search.HasErr() || strings.Join(titles, ",") != want {
			t.Errorf("search %q = %v, %v; want %s", text, titles, search.GetErr(), want)
		}
	}
}

func TestSearchFilter(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, tx = testaccount.New(t, ctx, tx)

	word := fmt.Sprintf("zq%d", time.Now().UnixNano())
	ids := map[string]string{}
	for _, content := range []string{
		"# Empty",
		"# Tagged\n\n#" + word,
		"# Long\n\n" + strings.Repeat("é", 40) + " " + word,
		"# Trashed",
	} {
		process := createnote.Process(ctx, tx, createnote.Input{Content: content})
		process.Run()
		if process.HasErr() {
			t.Fatalf("note %q: %v", content, process.GetErr())
		}
		ids[string(process.GetResult().Title)] = process.GetResult().ID
		if content == "# Trashed" {
			trash := deleteitems.Process(ctx, tx, domain.NOTE, []string{process.GetResult().ID})
			trash.Run()
			if trash.HasErr() {
				t.Fatalf("trash: %v", trash.GetErr())
			}
		}
	}

	// Lengths count characters, not bytes, title line included: "# Empty" is 7, "# Tagged\n\n#zq…" more than 20.
	tests := map[string]struct {
		text   string
		filter Filter
		want   string
	}{
		"untagged":            {"", Filter{Untagged: true}, "Empty,Long"},
		"short":               {"", Filter{MaxLength: 20}, "Empty"},
		"exact length":        {"", Filter{MaxLength: 7}, "Empty"},
		"multibyte in length": {"", Filter{MaxLength: 50 + len(word)}, "Empty,Long,Tagged"},
		"text and untagged":   {word, Filter{Untagged: true}, "Long"},
		"text and short":      {word, Filter{MaxLength: 30 + len(word)}, "Tagged"},
		// A title followed by nothing but #tags is an empty section.
		"empty sections":          {"", Filter{EmptySections: true}, "Empty,Tagged"},
		"text and empty sections": {word, Filter{EmptySections: true}, "Tagged"},
	}
	for name, test := range tests {
		search := Process(ctx, tx, test.text).Filter(test.filter)
		search.Run()
		var titles []string
		for _, match := range search.GetResult() {
			titles = append(titles, match.Title)
		}
		slices.Sort(titles)
		if search.HasErr() || strings.Join(titles, ",") != test.want || search.GetTotal() != len(titles) {
			t.Errorf("%s: titles = %v of %d, %v; want %s", name, titles, search.GetTotal(), search.GetErr(), test.want)
		}
	}

	empty := Process(ctx, tx, word).Filter(Filter{EmptySections: true})
	empty.Run()
	if result := empty.GetResult(); len(result) != 1 || !slices.Equal(result[0].EmptySections, []string{"# Tagged"}) {
		t.Errorf("empty sections of Tagged = %v", result)
	}

	// Stale notes: changed (or created) before the date, the oldest first without a text, ranked as usual with one.
	for title, date := range map[string]string{"Long": "2020-01-01", "Empty": "2021-01-01", "Trashed": "2019-01-01"} {
		if _, err := tx.NewRaw("UPDATE items SET created_at = ?, updated_at = ? WHERE id = ?", date, date, ids[title]).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for name, test := range map[string]struct {
		text   string
		filter Filter
		want   string
	}{
		"stale oldest first": {"", Filter{UpdatedBefore: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)}, "Long,Empty"},
		"stale before both":  {"", Filter{UpdatedBefore: time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)}, "Long"},
		"stale and untagged": {"", Filter{UpdatedBefore: time.Now().Add(time.Hour), Untagged: true}, "Long,Empty"},
		"stale and text":     {word, Filter{UpdatedBefore: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)}, "Long"},
	} {
		search := Process(ctx, tx, test.text).Filter(test.filter)
		search.Run()
		var titles []string
		for _, match := range search.GetResult() {
			titles = append(titles, match.Title)
		}
		if search.HasErr() || strings.Join(titles, ",") != test.want {
			t.Errorf("%s: titles = %v, %v; want %s", name, titles, search.GetErr(), test.want)
		}
	}

	// A page of a filter without a text skips the notes it drops: the second short or untagged note.
	page := Process(ctx, tx, "").Filter(Filter{Untagged: true, MaxLength: 100}).Page(1, 1)
	page.Run()
	if page.HasErr() || len(page.GetResult()) != 1 || page.GetTotal() != 2 {
		t.Errorf("page = %+v of %d, %v; want 1 of 2", page.GetResult(), page.GetTotal(), page.GetErr())
	}

	// The total of a max_length search counts the notes that pass it, whatever the page (#213): only "Empty" does.
	for _, offset := range []int{0, 1, 5} {
		page := Process(ctx, tx, "").Filter(Filter{MaxLength: 20}).Page(1, offset)
		page.Run()
		if page.HasErr() || page.GetTotal() != 1 {
			t.Errorf("offset %d: total = %d, %v; want 1", offset, page.GetTotal(), page.GetErr())
		}
	}
}

func TestSearchScope(t *testing.T) {
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	defer db.Close()

	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, tx = testaccount.New(t, ctx, tx)

	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Cuisine"})
	folder.Run()
	if folder.HasErr() {
		t.Fatalf("folder: %v", folder.GetErr())
	}
	sub := createfolder.Process(ctx, tx, createfolder.Input{Title: "Sub", ParentID: folder.GetResult().ID})
	sub.Run()
	if sub.HasErr() {
		t.Fatalf("subfolder: %v", sub.GetErr())
	}
	// The tags are written into the content as #tags.
	for _, note := range []struct {
		content, parent string
		tags            []string
	}{
		{"# Soup\n\nÉtapes: boil", folder.GetResult().ID, []string{"recette", "cuisine"}},
		{"# Cake\n\nflour", sub.GetResult().ID, []string{"recette", "idee"}},
		{"# Bare", sub.GetResult().ID, nil},
		{"# Tea", "", []string{"idee", "boisson"}},
		{"# Plan", "", []string{"idee"}},
		{"# Loose", "", nil},
	} {
		process := createnote.Process(ctx, tx, createnote.Input{Content: note.content, ParentID: note.parent, Tags: note.tags})
		process.Run()
		if process.HasErr() {
			t.Fatalf("note %q: %v", note.content, process.GetErr())
		}
	}
	var tags []domain.Tag
	if err := tx.NewSelect().Model(&tags).Where("user_id = ?", domain.UserID(ctx)).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	id := map[string]string{}
	for _, tag := range tags {
		id[string(tag.Value)] = tag.ID
	}

	// "Recette without Étapes", "without Recette nor Idée", "Idée AND (Cuisine OR Boisson)"; untagged and max_length
	// scoped to a folder and its subfolders.
	for name, test := range map[string]struct {
		text   string
		filter Filter
		want   string
	}{
		"all":                {"", Filter{Scope: Scope{TagsAll: []string{id["recette"], id["idee"]}}}, "Cake"},
		"all twice the same": {"", Filter{Scope: Scope{TagsAll: []string{id["idee"], id["idee"]}}}, "Cake,Plan,Tea"},
		"any":                {"", Filter{Scope: Scope{TagsAny: []string{id["cuisine"], id["boisson"]}}}, "Soup,Tea"},
		"all and any":        {"", Filter{Scope: Scope{TagsAll: []string{id["idee"]}, TagsAny: []string{id["cuisine"], id["boisson"]}}}, "Tea"},
		"none":               {"", Filter{Scope: Scope{TagsNone: []string{id["recette"], id["idee"]}}}, "Bare,Loose"},
		"folder":             {"", Filter{Scope: Scope{Folders: []string{folder.GetResult().ID}}}, "Bare,Cake,Soup"},
		"folder and none":    {"", Filter{Scope: Scope{Folders: []string{folder.GetResult().ID}, TagsNone: []string{id["recette"]}}}, "Bare"},
		"untagged in folder": {"", Filter{Scope: Scope{Folders: []string{folder.GetResult().ID}}, Untagged: true}, "Bare"},
		"short in subfolder": {"", Filter{Scope: Scope{Folders: []string{sub.GetResult().ID}}, MaxLength: 10}, "Bare"},
		"not text":           {"", Filter{Scope: Scope{TagsAll: []string{id["recette"]}}, NotText: "étapes"}, "Cake"},
		"not text anywhere":  {"", Filter{NotText: "A"}, "Loose"},
		"text and not text":  {"o", Filter{NotText: "soup"}, "Cake,Loose,Tea"},
	} {
		search := Process(ctx, tx, test.text).Filter(test.filter)
		search.Run()
		var titles []string
		for _, match := range search.GetResult() {
			titles = append(titles, match.Title)
		}
		slices.Sort(titles)
		if search.HasErr() || strings.Join(titles, ",") != test.want || search.GetTotal() != len(titles) {
			t.Errorf("%s: titles = %v of %d, %v; want %s", name, titles, search.GetTotal(), search.GetErr(), test.want)
		}
	}

	// A text left out opens every note: the total of a page is exact.
	page := Process(ctx, tx, "").Filter(Filter{NotText: "étapes"}).Page(1, 0)
	page.Run()
	if page.HasErr() || len(page.GetResult()) != 1 || page.GetTotal() != 5 {
		t.Errorf("page = %+v of %d, %v; want 1 of 5", page.GetResult(), page.GetTotal(), page.GetErr())
	}
}
