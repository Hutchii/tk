package tui

import (
	"fmt"
	"os"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/hutchii/tk/internal/store"
)

// TestDump writes plain-text screens to $TK_DUMP for eyeballing layout.
func TestDump(t *testing.T) {
	out := os.Getenv("TK_DUMP")
	if out == "" {
		t.Skip()
	}
	s := &store.Store{Dir: t.TempDir()}
	today := store.Today()
	add := func(title, p, plan, dl, done, body string) {
		x, _ := s.Add(title, p)
		x.Planned, x.Deadline, x.DoneAt, x.Body = plan, dl, done, body
		s.Save(&x)
	}
	add("Fix login redirect loop on Safari", "acme-web", today, store.AddDays(today, 1), "", "## Steps\n\n- reproduce\n- **fix** middleware\n\n```ts\nredirect('/login')\n```")
	add("Pricing page copy", "acme-web", store.AddDays(today, 1), "", "", "")
	add("Old slipped task", "events", store.AddDays(today, -2), store.AddDays(today, -1), "", "")
	add("Backlog idea", "bookings", "", "", "", "")
	add("Seo audit", "blog", "", store.AddDays(today, 9), "", "")
	add("Deploy env vars", "acme-web", today, "", today, "")
	add("Yesterday work", "events", store.AddDays(today, -1), "", store.AddDays(today, -1), "")
	add("Export csv", "bookings", store.AddDays(today, 2), "", "", "")

	m := newModel(s)
	m.w, m.h = 130, 20
	f, _ := os.Create(out)
	defer f.Close()
	shot := func(name string, mm model) {
		fmt.Fprintf(f, "===== %s\n%s\n", name, ansi.Strip(mm.View().Content))
	}
	shot("calendar", m)
	shot("calendar scrolled back", press(t, m, ","))
	shot("board all", press(t, m, "2"))
	shot("board acme-web done shown", press(t, m, "2", "h", "j", "j", "j", "d", "l"))
	shot("detail", press(t, m, "j", "enter"))
	shot("add prompt", typeText(t, press(t, m, "a"), "Thing @bookings ^fri"))
	m.w = 60
	shot("calendar narrow", m)
}
