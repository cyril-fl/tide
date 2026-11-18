package edituser

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/adduser"
	"github.com/cyril-fl/tide/core/services/process/createfile"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/silo"
	"github.com/cyril-fl/tide/core/services/testaccount"
)

// No DB needed: checkInput fails first and updateUser is guarded, so a nil db is never touched.
func TestInvalidInput(t *testing.T) {
	text := func(s string) *string { return &s }
	days := func(n int) *int { return &n }
	tests := []struct {
		input Input
		want  error
	}{
		{Input{Locale: text("")}, ErrInvalidLocale},
		{Input{Locale: text("de")}, ErrInvalidLocale},
		{Input{Locale: text("FR")}, ErrInvalidLocale},
		{Input{Theme: text("blue")}, ErrInvalidTheme},
		{Input{Theme: text("dark")}, ErrInvalidTheme},
		{Input{ColorMode: text("dim")}, ErrInvalidColorMode},
		{Input{View: text("table")}, ErrInvalidView},
		{Input{Sort: text("size")}, ErrInvalidSort},
		{Input{Sort: text("title,-title")}, ErrInvalidSort},
		{Input{AfterMerge: text("later")}, ErrInvalidAfterMerge},
		{Input{Plan: text("max")}, ErrInvalidPlan},
		{Input{TrashRetentionDays: days(-1)}, ErrInvalidRetention},
		{Input{TrashRetentionDays: days(MaxRetentionDays + 1)}, ErrInvalidRetention},
		{Input{NeighborsLimit: days(30)}, ErrInvalidNeighbors},
		{Input{Name: text(" ")}, adduser.ErrNoName},
		{Input{Snippets: &[]domain.Snippet{{Name: " ", Content: "x"}}}, ErrInvalidSnippet},
		{Input{Snippets: &[]domain.Snippet{{Name: "big", Content: string(make([]byte, MaxSnippetSize+1))}}}, ErrInvalidSnippet},
		{Input{Snippets: new(make([]domain.Snippet, MaxSnippets+1))}, ErrInvalidSnippet},
		{Input{SavedSearches: &[]domain.SavedSearch{{ID: "a", Name: " "}}}, ErrInvalidSavedSearch},
		{Input{SavedSearches: &[]domain.SavedSearch{{Name: "no id"}}}, ErrInvalidSavedSearch},
		{Input{SavedSearches: &[]domain.SavedSearch{{ID: "a", Name: "x", TagMode: "both"}}}, ErrInvalidSavedSearch},
		{Input{SavedSearches: &[]domain.SavedSearch{{ID: "a", Name: "x", FolderMode: "maybe"}}}, ErrInvalidSavedSearch},
		{Input{SavedSearches: &[]domain.SavedSearch{{ID: "a", Name: "x", TagIDs: []string{"t1"}, AnyTagIDs: []string{"t2"}}}}, ErrInvalidSavedSearch},
		{Input{SavedSearches: &[]domain.SavedSearch{{ID: "a", Name: "x", NoTagIDs: make([]string, MaxSavedSearchIDs+1)}}}, ErrInvalidSavedSearch},
		{Input{SidebarSections: &[]domain.SidebarSection{{ID: "graph"}}}, ErrInvalidSidebar},
		{Input{SidebarSections: &[]domain.SidebarSection{{ID: "recent"}, {ID: "recent"}}}, ErrInvalidSidebar},
	}
	for _, test := range tests {
		process := Process(context.Background(), nil, test.input)
		process.Run()

		if !errors.Is(process.GetErr(), test.want) {
			t.Errorf("%+v: err = %v, want %v", test.input, process.GetErr(), test.want)
		}
	}
}

func TestEditUser(t *testing.T) {
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

	locale, view, sort, never, plan, keep := "en", "grid", "pinned,-updated", 0, "pro", "keep"
	process := Process(ctx, tx, Input{Locale: &locale})
	process.Run()
	if process.HasErr() {
		t.Fatalf("edit: %v", process.GetErr())
	}
	if got := process.GetResult(); got.Locale != "en" || got.ID == "" || got.Theme != "notes" || got.ColorMode != "system" || got.TrashRetentionDays != 30 || got.NeighborsLimit != 50 || got.Plan != "base" || got.AfterMerge != "ask" || got.Sort != "" {
		t.Errorf("result = %+v, want locale en with ID and the default settings", got)
	}

	process = Process(ctx, tx, Input{View: &view, Sort: &sort, TrashRetentionDays: &never, NeighborsLimit: &never, Plan: &plan, AfterMerge: &keep})
	process.Run()
	if process.HasErr() {
		t.Fatalf("edit: %v", process.GetErr())
	}
	if got := process.GetResult(); got.Locale != "en" || got.View != "grid" || got.Sort != "pinned,-updated" || got.TrashRetentionDays != 0 || got.NeighborsLimit != 0 || got.Plan != "pro" || got.AfterMerge != "keep" {
		t.Errorf("result = %+v, want locale kept, view grid, sort pinned,-updated, retention 0, every neighbor, plan pro, forks kept", got)
	}

	if got := process.GetResult(); len(got.Snippets) != 0 {
		t.Errorf("snippets = %+v, want none by default", got.Snippets)
	}
	snippets := []domain.Snippet{{Name: "Meeting", Content: "## Meeting #work"}, {Name: "Day", Content: "# Today"}}
	process = Process(ctx, tx, Input{Snippets: &snippets})
	process.Run()
	if got := process.GetResult(); process.HasErr() || len(got.Snippets) != 2 || got.Snippets[1] != snippets[1] {
		t.Errorf("snippets: result = %+v, err = %v; want both saved", got.Snippets, process.GetErr())
	}

	searches := []domain.SavedSearch{{ID: "s1", Name: "Work", TagMode: "all"}, {ID: "s2", Name: "Recent"}}
	process = Process(ctx, tx, Input{SavedSearches: &searches})
	process.Run()
	if got := process.GetResult(); process.HasErr() || len(got.SavedSearches) != 2 || got.SavedSearches[0].Name != "Work" || got.SavedSearches[0].TagMode != "all" {
		t.Errorf("saved searches: result = %+v, err = %v; want both saved", got.SavedSearches, process.GetErr())
	}

	sections := []domain.SidebarSection{{ID: "folders", Shown: false}, {ID: "recent", Shown: true}}
	process = Process(ctx, tx, Input{SidebarSections: &sections})
	process.Run()
	if got := process.GetResult(); process.HasErr() || !slices.Equal(got.SidebarSections, sections) {
		t.Errorf("sidebar: result = %+v, err = %v; want %+v", got.SidebarSections, process.GetErr(), sections)
	}

	process = Process(ctx, tx, Input{})
	process.Run()
	if got := process.GetResult(); process.HasErr() || got.View != "grid" || len(got.Snippets) != 2 {
		t.Errorf("empty edit: result = %+v, err = %v; want the account unchanged", got, process.GetErr())
	}

	// A column the model does not know, like one left by an old version, is not read back.
	if _, err := tx.ExecContext(ctx, "ALTER TABLE users ADD COLUMN legacy boolean NOT NULL DEFAULT false"); err != nil {
		t.Fatal(err)
	}
	process = Process(ctx, tx, Input{View: &view})
	process.Run()
	if process.HasErr() {
		t.Errorf("edit with an unknown column: %v", process.GetErr())
	}

	// The name of another account is taken, whatever its case; the savepoint keeps tx usable.
	var other domain.User
	if err := tx.NewSelect().Model(&other).Where("id <> ?", domain.UserID(ctx)).Limit(1).Scan(ctx); err != nil {
		other = domain.User{Name: "edituser-other", PasswordHash: "-"}
		if _, err := tx.NewInsert().Model(&other).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	taken := " " + other.Name + " "
	process = Process(ctx, tx, Input{Name: &taken})
	process.Run()
	if !errors.Is(process.GetErr(), adduser.ErrNameTaken) {
		t.Errorf("taken name: err = %v, want ErrNameTaken", process.GetErr())
	}
	name := " renamed-" + other.Name + " "
	process = Process(ctx, tx, Input{Name: &name})
	process.Run()
	if got := process.GetResult(); process.HasErr() || got.Name != "renamed-"+other.Name {
		t.Errorf("rename: result = %+v, err = %v; want the name trimmed", got, process.GetErr())
	}

	// An avatar is an image of the account: not a stranger's id, not a non-uuid, not a PDF.
	for _, bad := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		process = Process(ctx, tx, Input{Avatar: &bad})
		process.Run()
		if !errors.Is(process.GetErr(), ErrInvalidAvatar) {
			t.Errorf("avatar %q: err = %v, want ErrInvalidAvatar", bad, process.GetErr())
		}
	}
	file := createfile.Process(ctx, tx, silo.NewMemory(), createfile.Input{Name: "me.png", Data: []byte("\x89PNG\r\n\x1a\n")})
	file.Run()
	if file.HasErr() {
		t.Fatal(file.GetErr())
	}
	pdf := createfile.Process(ctx, tx, silo.NewMemory(), createfile.Input{Name: "me.pdf", Data: []byte("%PDF-1.7")})
	pdf.Run()
	if pdf.HasErr() {
		t.Fatal(pdf.GetErr())
	}
	pdfID := pdf.GetResult().ID
	process = Process(ctx, tx, Input{Avatar: &pdfID})
	process.Run()
	if !errors.Is(process.GetErr(), ErrInvalidAvatar) {
		t.Errorf("pdf avatar: err = %v, want ErrInvalidAvatar", process.GetErr())
	}
	avatar, none := file.GetResult().ID, ""
	process = Process(ctx, tx, Input{Avatar: &avatar})
	process.Run()
	if got := process.GetResult(); process.HasErr() || got.Avatar != avatar {
		t.Errorf("avatar: result = %+v, err = %v; want %s", got, process.GetErr(), avatar)
	}
	process = Process(ctx, tx, Input{Avatar: &none})
	process.Run()
	if got := process.GetResult(); process.HasErr() || got.Avatar != "" {
		t.Errorf("no avatar: result = %+v, err = %v; want none", got, process.GetErr())
	}
}

func TestEditUserGuards(t *testing.T) {
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

	searches := []domain.SavedSearch{{ID: "s1", Name: "Work"}}
	process := Process(ctx, tx, Input{SavedSearches: &searches})
	process.Run()
	if !errors.Is(process.GetErr(), ErrNoSavedSearches) {
		t.Errorf("saved searches on base: err = %v, want ErrNoSavedSearches", process.GetErr())
	}

	pro := "pro"
	process = Process(ctx, tx, Input{Plan: &pro, SavedSearches: &searches})
	process.Run()
	if process.HasErr() {
		t.Fatalf("saved searches with pro: %v", process.GetErr())
	}
	read := process.GetResult().UpdatedAt

	snippets := []domain.Snippet{{Name: "A"}}
	process = Process(ctx, tx, Input{Snippets: &snippets, UpdatedAt: &read})
	process.Run()
	if process.HasErr() || process.GetResult().UpdatedAt.Equal(read) {
		t.Fatalf("write over the version read: err = %v, updated_at = %v; want saved and bumped", process.GetErr(), process.GetResult().UpdatedAt)
	}

	other := []domain.Snippet{{Name: "B"}}
	process = Process(ctx, tx, Input{Snippets: &other, UpdatedAt: &read})
	process.Run()
	if !errors.Is(process.GetErr(), ErrStale) {
		t.Errorf("write over an older version: err = %v, want ErrStale", process.GetErr())
	}
	var user domain.User
	if err := tx.NewSelect().Model(&user).Where("id = ?", domain.UserID(ctx)).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if len(user.Snippets) != 1 || user.Snippets[0].Name != "A" {
		t.Errorf("snippets = %+v, want the first write kept", user.Snippets)
	}
}
