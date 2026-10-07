package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPlace(t *testing.T) {
	today := "2026-10-06"
	cases := []struct {
		name string
		task Task
		want Placement
	}{
		{"backlog", Task{}, Placement{}},
		{"planned today", Task{Planned: today}, Placement{Day: today}},
		{"planned future", Task{Planned: "2026-10-09"}, Placement{Day: "2026-10-09"}},
		{"slipped plan carries to today", Task{Planned: "2026-10-04"}, Placement{Day: today, CarriedFrom: "2026-10-04"}},
		{"done late sits on done day", Task{Planned: "2026-10-04", DoneAt: "2026-10-05"}, Placement{Day: "2026-10-05"}},
		{"done early sits on done day", Task{Planned: "2026-10-09", DoneAt: today}, Placement{Day: today}},
		{"done without plan", Task{DoneAt: "2026-10-01"}, Placement{Day: "2026-10-01"}},
	}
	for _, c := range cases {
		if got := Place(c.task, today); got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestDeadline(t *testing.T) {
	today := "2026-10-06"
	cases := []struct {
		task Task
		want DeadlineState
	}{
		{Task{}, DeadlineNone},
		{Task{Deadline: "2026-10-05"}, DeadlinePassed},
		{Task{Deadline: "2026-10-05", DoneAt: today}, DeadlineNone},
		{Task{Deadline: today}, DeadlineSoon},
		{Task{Deadline: "2026-10-08"}, DeadlineSoon},
		{Task{Deadline: "2026-10-09"}, DeadlineLater},
	}
	for _, c := range cases {
		if got := Deadline(c.task, today); got != c.want {
			t.Errorf("deadline %q done %q: got %v want %v", c.task.Deadline, c.task.DoneAt, got, c.want)
		}
	}
}

func TestParseDay(t *testing.T) {
	today := "2026-10-06" // a Tuesday
	cases := map[string]string{
		"t": today, "m": "2026-10-07", "+3": "2026-10-09", "-1": "2026-10-05",
		"fri": "2026-10-09", "tue": "2026-10-13", "monday": "2026-10-12",
		"2026-12-01": "2026-12-01", "11-02": "2026-11-02", "": "", "none": "",
	}
	for in, want := range cases {
		got, err := ParseDay(in, today)
		if err != nil || got != want {
			t.Errorf("ParseDay(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseDay("someday", today); err == nil {
		t.Error("ParseDay(someday) should fail")
	}
}

func TestRoundTripAndRename(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	task, err := s.Add("Fix login: redirect loop", "acme-web")
	if err != nil {
		t.Fatal(err)
	}
	task.Body = "## Steps\n\n- one\n- two\n\n---\n\nafter a rule"
	task.Deadline = "2026-10-10"
	if err := s.Save(&task); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != task.Body || got.Deadline != "2026-10-10" || got.Title != task.Title {
		t.Fatalf("round trip lost data: %+v", got)
	}

	oldPath := got.Path
	got.Project = "events"
	got.Title = "Renamed"
	if err := s.Save(&got); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file still there: %s", oldPath)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "acme-web")); !os.IsNotExist(err) {
		t.Errorf("empty old project dir left behind")
	}
	if !strings.HasSuffix(got.Path, filepath.Join("events", task.ID+"-renamed.md")) {
		t.Errorf("unexpected path %s", got.Path)
	}
	all, err := s.All()
	if err != nil || len(all) != 1 {
		t.Fatalf("All = %d tasks, %v", len(all), err)
	}
}

func TestAddRejectsBadProject(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	for _, p := range []string{"", "../etc", "a/b"} {
		if _, err := s.Add("x", p); err == nil {
			t.Errorf("project %q accepted", p)
		}
	}
}

// Review F1: on a case-insensitive disk a case-only project change used to
// write the file and then delete it as the "old" path.
func TestProjectCaseDoesNotDelete(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	a, _ := s.Add("write report", "Work")
	b, _ := s.Add("call bank", "work")
	if _, err := s.Update(a.ID, func(t *Task) error { t.DoneAt = Today(); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(b.ID, func(t *Task) error { t.Project = "WORK"; return nil }); err != nil {
		t.Fatal(err)
	}
	all, _ := s.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 tasks, have %d", len(all))
	}
	for _, x := range all {
		if x.Project != "work" {
			t.Errorf("project not folded: %q", x.Project)
		}
	}
}

// Review F2/F5: parallel writers must not share ids or lose updates.
func TestConcurrentWriters(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.Add(fmt.Sprintf("task %d", i), "p"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	all, _ := s.All()
	ids := map[string]bool{}
	for _, x := range all {
		ids[x.ID] = true
	}
	if len(all) != 20 || len(ids) != 20 {
		t.Fatalf("%d files, %d unique ids; want 20/20", len(all), len(ids))
	}

	id := all[0].ID
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.Update(id, func(t *Task) error { t.Planned = "2026-10-09"; return nil })
		}()
		go func(i int) {
			defer wg.Done()
			s.Update(id, func(t *Task) error { t.Title = fmt.Sprintf("renamed %d", i); t.DoneAt = "2026-10-06"; return nil })
		}(i)
	}
	wg.Wait()
	got, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Planned != "2026-10-09" || got.DoneAt != "2026-10-06" {
		t.Fatalf("lost update: %+v", got)
	}
	if m, _ := filepath.Glob(filepath.Join(s.Dir, "*", id+"-*")); len(m) != 1 {
		t.Fatalf("id has %d files: %v", len(m), m)
	}
}

// Review F4 and F8: unknown keys and body indentation survive a save.
func TestKeepsExtraFieldsAndIndent(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	os.MkdirAll(filepath.Join(s.Dir, "p"), 0o755)
	raw := "---\nid: abc123\ntitle: x\nproject: p\npriority: high\ntags: [a, b]\ncreated: \"2026-10-01\"\n---\n    $ make deploy\n    $ make check\n"
	os.WriteFile(filepath.Join(s.Dir, "p", "abc123-x.md"), []byte(raw), 0o644)
	if _, err := s.Update("abc123", func(t *Task) error { t.DoneAt = "2026-10-06"; return nil }); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(s.Dir, "p", "abc123-x.md"))
	for _, want := range []string{"priority: high", "tags:", "    $ make deploy\n    $ make check\n", "done_at:"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// Review F9/F10: ids are never globs or paths.
func TestBadIDsRejected(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	s.Add("only task", "p")
	for _, id := range []string{"", "*", "?", "[a-z]*", "../../x", "a/b"} {
		if _, err := s.Delete(id); err == nil {
			t.Errorf("Delete(%q) accepted", id)
		}
	}
	if all, _ := s.All(); len(all) != 1 {
		t.Fatal("a bad id deleted the task")
	}
}

// Post fields are omitempty: saving a task written before they existed must
// not change its file.
func TestTaskFileUnchangedByPostFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acme", "abc-fix-login.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	orig := "---\nid: abc\ntitle: Fix login\nproject: acme\ndeadline: \"2026-10-10\"\nplanned: \"2026-10-07\"\ncreated: \"2026-10-06\"\n---\nBody\n"
	os.WriteFile(path, []byte(orig), 0o644)
	s := &Store{Dir: dir}
	if _, err := s.Update("abc", func(*Task) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != orig {
		t.Fatalf("file changed:\n%s", got)
	}
}

func TestSlugTransliteratesPolish(t *testing.T) {
	if got := slug("Replika, jak czytelnik szuka książki"); got != "replika-jak-czytelnik-szuka-ksiazki" {
		t.Fatalf("got %q", got)
	}
}
