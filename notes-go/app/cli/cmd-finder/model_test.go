package cmdfinder

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient"
)

// tree: root holds folder A (with note N inside) and note R.
var (
	rootID    = ""
	folderA   = apiclient.DomainItem{Id: "a", Kind: apiclient.FOLDER, Title: "A"}
	noteR     = apiclient.DomainItem{Id: "r", Kind: apiclient.NOTE, Title: "Readme"}
	noteN     = apiclient.DomainItem{Id: "n", Kind: apiclient.NOTE, Title: "Notes", ParentId: &folderA.Id}
	folderOfA = page{ReaditemResult: apiclient.ReaditemResult{Item: apiclient.DomainItem{Id: "a", Kind: apiclient.FOLDER, Title: "A", ParentId: &rootID}, Path: "/A"}, children: []apiclient.DomainItem{noteN}}
	root      = page{ReaditemResult: apiclient.ReaditemResult{Item: apiclient.DomainItem{Kind: apiclient.FOLDER, Title: "Notes"}, Path: "/"}, children: []apiclient.DomainItem{folderA, noteR}, tags: map[string][]string{"r": {"doc"}}}
)

func fakeLoad(id string) (page, error) {
	if id == "a" {
		return folderOfA, nil
	}
	return root, nil
}

// press sends a key and runs the command it returns, like the bubbletea loop would.
func press(m *model, key tea.KeyPressMsg) tea.Cmd {
	_, cmd := m.Update(key)
	if cmd != nil {
		if msg, ok := cmd().(loadedMsg); ok {
			m.Update(msg)
			return nil
		}
	}
	return cmd
}

var (
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	up    = tea.KeyPressMsg{Code: tea.KeyUp}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	left  = tea.KeyPressMsg{Code: tea.KeyLeft}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	ctrl  = func(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
	text  = func(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }
)

func TestNavigate(t *testing.T) {
	m := newModel(fakeLoad, root, 0, nil, "", false)

	press(m, up)
	press(m, down)
	press(m, down)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (bounded)", m.cursor)
	}

	press(m, up)
	press(m, enter)
	if m.folder.Path != "/A" || m.cursor != 0 {
		t.Fatalf("after enter: path = %q, cursor = %d; want /A, 0", m.folder.Path, m.cursor)
	}

	press(m, left)
	if m.folder.Path != "/" || m.cursor != 0 {
		t.Errorf("after back: path = %q, cursor = %d; want /, cursor on A", m.folder.Path, m.cursor)
	}

	press(m, enter)
	press(m, esc) // Esc goes back too
	if m.folder.Path != "/" || m.action.kind != actionQuit {
		t.Errorf("after esc in /A: path = %q, action = %v; want /, still running", m.folder.Path, m.action.kind)
	}

	press(m, left) // no-op at the root
	if m.folder.Path != "/" || m.action.kind != actionQuit {
		t.Errorf("back at root: path = %q, action = %v", m.folder.Path, m.action.kind)
	}
}

func TestActions(t *testing.T) {
	tests := map[string]struct {
		cursor int
		key    tea.KeyPressMsg
		want   action
	}{
		"enter on note opens": {1, enter, action{actionOpen, noteR}},
		"ctrl+e edits":        {0, ctrl('e'), action{actionEdit, folderA}},
		"ctrl+n creates":      {0, ctrl('n'), action{actionCreate, apiclient.DomainItem{}}},
		"ctrl+d deletes":      {1, ctrl('d'), action{actionDelete, noteR}},
		"ctrl+q quits":        {0, ctrl('q'), action{actionQuit, apiclient.DomainItem{}}},
	}

	for name, test := range tests {
		m := newModel(fakeLoad, root, test.cursor, nil, "", false)
		if cmd := press(m, test.key); cmd == nil {
			t.Errorf("%s: no quit command", name)
		}
		if m.action.kind != test.want.kind || m.action.item.Id != test.want.item.Id {
			t.Errorf("%s: action = %+v, want %+v", name, m.action, test.want)
		}
	}
}

func TestFilter(t *testing.T) {
	m := newModel(fakeLoad, root, 0, nil, "", false)

	press(m, text('/'))
	for _, r := range "READ" {
		press(m, text(r))
	}
	if got := m.visible(); len(got) != 1 || got[0].Id != noteR.Id {
		t.Fatalf("visible = %+v, want [Readme]", got)
	}

	press(m, enter) // keeps the filter, leaves typing
	if m.filtering || m.filter.Value() == "" {
		t.Fatalf("filtering = %v, filter = %q; want kept, not typing", m.filtering, m.filter.Value())
	}

	// At the root, the first Esc clears the filter, the second quits.
	if press(m, esc); m.filter.Value() != "" || m.action.kind != actionQuit || len(m.visible()) != 2 {
		t.Fatalf("after esc: filter = %q, visible = %d", m.filter.Value(), len(m.visible()))
	}
	if cmd := press(m, esc); cmd == nil {
		t.Error("second esc does not quit")
	}
}

func TestFilterTags(t *testing.T) {
	m := newModel(fakeLoad, root, 0, nil, "", false)

	for _, query := range []string{"doc", "#DOC"} {
		m.filter.SetValue(query)
		if got := m.visible(); len(got) != 1 || got[0].Id != noteR.Id {
			t.Errorf("%q: visible = %+v, want [Readme] by its tag", query, got)
		}
	}
}

func TestScroll(t *testing.T) {
	folder := page{ReaditemResult: apiclient.ReaditemResult{Path: "/"}}
	for i := 0; i < 20; i++ {
		folder.children = append(folder.children, apiclient.DomainItem{Id: string(rune('a' + i)), Kind: apiclient.NOTE, Title: string(rune('a' + i))})
	}
	m := newModel(fakeLoad, folder, 0, nil, "", false)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12}) // 12 - 1 - 3 - 2 = 6 rows

	for i := 0; i < 10; i++ {
		press(m, down)
	}
	m.View()
	if m.cursor < m.offset || m.cursor >= m.offset+m.rows() {
		t.Errorf("cursor %d outside the window [%d, %d)", m.cursor, m.offset, m.offset+m.rows())
	}
}

// Regression: down in an empty folder set the cursor to -1 and View panicked on children[-1:].
func TestEmptyFolder(t *testing.T) {
	m := newModel(fakeLoad, page{ReaditemResult: apiclient.ReaditemResult{Path: "/"}}, 0, nil, "", false)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})

	press(m, down)
	press(m, up)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	m.View()

	if press(m, enter); m.action.kind != actionQuit {
		t.Errorf("enter on nothing: action = %v, want none", m.action.kind)
	}
}

func TestCutPaste(t *testing.T) {
	m := newModel(fakeLoad, root, 1, nil, "", false)

	if press(m, ctrl('v')); m.action.kind != actionQuit {
		t.Fatalf("paste with nothing cut: action = %v, want none", m.action.kind)
	}

	press(m, ctrl('x')) // cut Readme
	if m.clipboard == nil || m.clipboard.Id != noteR.Id {
		t.Fatalf("clipboard = %+v, want Readme", m.clipboard)
	}

	press(m, esc) // cancels the cut, does not quit
	if m.clipboard != nil || m.action.kind != actionQuit {
		t.Fatalf("after esc: clipboard = %+v, action = %v", m.clipboard, m.action.kind)
	}

	press(m, ctrl('x'))
	press(m, up)
	press(m, enter) // into A, the clipboard follows
	if cmd := press(m, ctrl('v')); cmd == nil || m.action.kind != actionMove || m.action.item.Id != noteR.Id || m.folder.Path != "/A" {
		t.Errorf("paste: action = %+v, folder = %q; want move Readme into /A", m.action, m.folder.Path)
	}
}
