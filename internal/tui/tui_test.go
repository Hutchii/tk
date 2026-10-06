package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hutchii/tk/internal/store"
)

func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		default:
			r := []rune(k)
			msg = tea.KeyPressMsg{Code: r[0], Text: k}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

func typeText(t *testing.T, m model, s string) model {
	for _, r := range s {
		m = press(t, m, string(r))
	}
	return m
}

func seed(t *testing.T) (*store.Store, store.Task) {
	s := &store.Store{Dir: t.TempDir()}
	a, _ := s.Add("Fix login", "acme-web")
	a.Planned = store.Today()
	if err := s.Save(&a); err != nil {
		t.Fatal(err)
	}
	return s, a
}

func reread(t *testing.T, s *store.Store, id string) store.Task {
	t.Helper()
	got, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCalendarMoveAndDone(t *testing.T) {
	s, a := seed(t)
	today := store.Today()
	m := newModel(s)
	if cur, ok := m.current(); !ok || cur.ID != a.ID {
		t.Fatalf("cursor not on today's task: %+v", cur)
	}

	m = press(t, m, "]")
	if got := reread(t, s, a.ID); got.Planned != store.AddDays(today, 1) {
		t.Fatalf("] planned = %q", got.Planned)
	}
	if m.col != 2 {
		t.Errorf("cursor did not follow the task, col %d", m.col)
	}

	m = press(t, m, "[", "[")
	if got := reread(t, s, a.ID); got.Planned != "" {
		t.Fatalf("[ from today should go to backlog, planned = %q", got.Planned)
	}
	if m.col != 0 {
		t.Errorf("cursor not in backlog, col %d", m.col)
	}

	m = press(t, m, "]", "x")
	got := reread(t, s, a.ID)
	if got.DoneAt != today || got.Planned != today {
		t.Fatalf("x: done_at %q planned %q", got.DoneAt, got.Planned)
	}
	m = press(t, m, "]")
	if reread(t, s, a.ID).Planned != today {
		t.Error("] moved a done task")
	}
	m = press(t, m, "x")
	if reread(t, s, a.ID).DoneAt != "" {
		t.Error("second x did not undo")
	}
}

func TestAddPrompt(t *testing.T) {
	s, _ := seed(t)
	m := newModel(s)
	m = press(t, m, "a")
	m = typeText(t, m, "Ship pricing @bookings ^+2 !+5")
	m = press(t, m, "enter")
	files, _ := filepath.Glob(filepath.Join(s.Dir, "bookings", "*-ship-pricing.md"))
	if len(files) != 1 {
		t.Fatalf("expected one new file, got %v (status %q)", files, m.status)
	}
	raw, _ := os.ReadFile(files[0])
	today := store.Today()
	for _, want := range []string{"title: Ship pricing", "planned: \"" + store.AddDays(today, 2), "deadline: \"" + store.AddDays(today, 5)} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("file missing %q:\n%s", want, raw)
		}
	}
	if cur, _ := m.current(); cur.Title != "Ship pricing" {
		t.Errorf("cursor not on new task: %q", cur.Title)
	}

	// No @project and cursor on a acme-web task: inherits acme-web, planned on the cursor's day.
	m = press(t, m, "g")
	m = press(t, m, "a")
	m = typeText(t, m, "Second")
	m = press(t, m, "enter")
	files, _ = filepath.Glob(filepath.Join(s.Dir, "acme-web", "*-second.md"))
	if len(files) != 1 {
		t.Fatalf("default project not applied: status %q", m.status)
	}
	raw, _ = os.ReadFile(files[0])
	if !strings.Contains(string(raw), "planned: \""+today) {
		t.Errorf("default day not applied:\n%s", raw)
	}
}

func TestBoardAndRender(t *testing.T) {
	s, a := seed(t)
	done, _ := s.Add("Old work", "acme-web")
	done.DoneAt = store.AddDays(store.Today(), -3)
	s.Save(&done)

	m := newModel(s)
	m = press(t, m, "2")
	if m.view != viewBoard {
		t.Fatal("2 did not open board")
	}
	if n := len(m.boardList()); n != 1 {
		t.Errorf("done should be hidden, list has %d", n)
	}
	m = press(t, m, "d")
	if n := len(m.boardList()); n != 2 {
		t.Errorf("d should show done, list has %d", n)
	}
	if cur, _ := m.current(); cur.ID != a.ID {
		t.Errorf("board cursor lost the task")
	}

	// 0x0 is what a pty without a size reports; it used to panic in strings.Repeat.
	for _, w := range []int{0, 5, 24, 25, 30, 80, 140, 220} {
		for _, v := range []string{"1", "2", "?"} {
			m.w, m.h = w, 24
			if w < 10 {
				m.h = w
			}
			mm := press(t, m, v)
			out := ansi.Strip(mm.View().Content)
			lines := strings.Split(out, "\n")
			if len(lines) > max(m.h, 1) {
				t.Errorf("w=%d view %s: %d lines, taller than screen", w, v, len(lines))
			}
			for i, l := range lines {
				if lw := ansi.StringWidth(l); lw > w {
					t.Errorf("w=%d view %s line %d is %d wide: %q", w, v, i, lw, l)
					break
				}
			}
		}
	}
}

// An edit made outside the app between reloads must survive a keypress.
func TestKeypressKeepsExternalEdit(t *testing.T) {
	s, a := seed(t)
	m := newModel(s)
	ext := reread(t, s, a.ID)
	ext.Body = "written by claude"
	if err := s.Save(&ext); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, "x")
	got := reread(t, s, a.ID)
	if got.Body != "written by claude" || got.DoneAt == "" {
		t.Fatalf("body %q done %q", got.Body, got.DoneAt)
	}
	m = press(t, m, "x", "]")
	if reread(t, s, a.ID).Body != "written by claude" {
		t.Fatal("] dropped the external edit")
	}
}

// Review F6: a description changed on disk while the editor was open is kept,
// and the user's text stays in the temp file.
func TestEditorConflictKeepsBoth(t *testing.T) {
	s, a := seed(t)
	m := newModel(s)
	tmp := filepath.Join(t.TempDir(), "edit.md")
	os.WriteFile(tmp, []byte("v1 from user"), 0o644)
	s.Update(a.ID, func(x *store.Task) error { x.Body = "v2 from claude"; return nil })
	next, _ := m.Update(editorDoneMsg{id: a.ID, path: tmp, orig: ""})
	if got := reread(t, s, a.ID).Body; got != "v2 from claude" {
		t.Fatalf("body = %q", got)
	}
	if b, err := os.ReadFile(tmp); err != nil || string(b) != "v1 from user" {
		t.Fatalf("user text lost: %q %v", b, err)
	}
	if st := next.(model).status; !strings.Contains(st, tmp) {
		t.Errorf("status does not point at the temp file: %q", st)
	}
}

// Review F7: an editor error keeps the temp file and does not save.
func TestEditorErrorKeepsText(t *testing.T) {
	s, a := seed(t)
	m := newModel(s)
	tmp := filepath.Join(t.TempDir(), "edit.md")
	os.WriteFile(tmp, []byte("typed text"), 0o644)
	m.Update(editorDoneMsg{id: a.ID, path: tmp, err: os.ErrClosed})
	if _, err := os.Stat(tmp); err != nil {
		t.Fatal("temp file removed after editor error")
	}
	if reread(t, s, a.ID).Body != "" {
		t.Fatal("saved despite editor error")
	}
	m.Update(editorDoneMsg{id: a.ID, path: tmp, orig: ""})
	if reread(t, s, a.ID).Body != "typed text" {
		t.Fatal("normal save failed")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file left after a clean save")
	}
}
