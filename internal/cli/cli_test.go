package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hutchii/tk/internal/store"
)

func run(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := Run(args, strings.NewReader(stdin), &out)
	return out.String(), err
}

func onlyTask(t *testing.T, dir string) store.Task {
	t.Helper()
	all, err := (&store.Store{Dir: dir}).All()
	if err != nil || len(all) != 1 {
		t.Fatalf("want 1 task, have %d (%v)", len(all), err)
	}
	return all[0]
}

func TestAddKeepsDashWordsAndRejectsBadDay(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TK_DIR", dir)
	// Review F3: a bad day fails before anything is written.
	if _, err := run(t, "", "add", "x", "-p", "a", "--plan", "someday"); err == nil {
		t.Fatal("bad --plan accepted")
	}
	// Review F11: "-v" is part of the title, not a flag.
	if _, err := run(t, "", "add", "Fix", "-v", "flag", "-p", "a", "--plan", "-1"); err != nil {
		t.Fatal(err)
	}
	got := onlyTask(t, dir)
	if got.Title != "Fix -v flag" || got.Planned != store.AddDays(store.Today(), -1) {
		t.Fatalf("got title %q planned %q", got.Title, got.Planned)
	}
}

// Review F12: empty stdin must not wipe a description.
func TestEmptyStdinKeepsDescription(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TK_DIR", dir)
	if _, err := run(t, "keep me", "add", "x", "-p", "a", "--desc", "-"); err != nil {
		t.Fatal(err)
	}
	id := onlyTask(t, dir).ID
	if _, err := run(t, "", "edit", id, "--desc", "-"); err == nil {
		t.Fatal("empty stdin accepted")
	}
	if b := onlyTask(t, dir).Body; b != "keep me" {
		t.Fatalf("body = %q", b)
	}
	if _, err := run(t, "", "edit", id, "--desc", ""); err != nil {
		t.Fatal(err)
	}
	if b := onlyTask(t, dir).Body; b != "" {
		t.Fatalf(`--desc "" did not clear: %q`, b)
	}
}
