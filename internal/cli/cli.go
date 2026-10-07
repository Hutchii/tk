// Package cli is the command interface Claude (and scripts) use. It goes
// through the same store as the TUI so dates and file layout stay consistent.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/hutchii/tk/internal/store"
)

const usage = `tk - terminal task planner

  tk                         open the app
  tk today [--json]          today's plan: due today, carried over, done today
  tk day <day> [--json]      one day (t, m, +3, fri, 2026-10-12)
  tk list [-p project] [--done|--all] [--json]
  tk show <id> [--json]      full task with description and repo path
  tk add <title> -p <project> [--plan <day>] [--deadline <day>] [--desc <text>|--desc -]
  tk plan <id> <day|none>
  tk deadline <id> <day|none>
  tk done <id> [--on <day>]  mark done today, or on that day
  tk undo <id>               mark not done
  tk edit <id> [--title t] [-p project] [--desc <text>|--desc -]
  tk rm <id>
  tk projects [--json]

--desc - reads the description from stdin. Data lives in $TK_DIR or ~/tasks.

Posts: "tk post <command>" runs any command above on the post calendar in
$TK_SOCIAL_DIR or ~/social; "tk post" alone opens it. Planned = the day it goes
out, done = published that day. Posts also take:
  --platform linkedin|instagram|facebook|tiktok   (add, edit)
  --status draft|approved                         (add, edit)
  --url <link>                                    (edit, done)
`

var platforms = map[string]bool{"linkedin": true, "instagram": true, "facebook": true, "tiktok": true}
var statuses = map[string]bool{"draft": true, "approved": true}

type out struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Project     string `json:"project"`
	Platform    string `json:"platform,omitempty"`
	Status      string `json:"status,omitempty"`
	URL         string `json:"url,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Planned     string `json:"planned,omitempty"`
	Deadline    string `json:"deadline,omitempty"`
	DoneAt      string `json:"done_at,omitempty"`
	CarriedFrom string `json:"carried_from,omitempty"`
	File        string `json:"file"`
	Description string `json:"description,omitempty"`
}

func Run(args []string, stdin io.Reader, stdout io.Writer) error {
	open := store.Open
	if args[0] == "post" {
		open, args = store.OpenSocial, args[1:]
		if len(args) == 0 {
			return errors.New("usage: tk post <command>; tk post alone opens the app")
		}
	}
	s, err := open()
	if err != nil {
		return err
	}
	cmd, rest := args[0], args[1:]
	pos, flags, err := parse(rest)
	if err != nil {
		return err
	}
	asJSON := flags["json"] != ""
	today := store.Today()

	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return nil

	case "today", "day":
		day := today
		if cmd == "day" {
			if len(pos) != 1 {
				return errors.New("usage: tk day <day>")
			}
			if day, err = store.ParseDay(pos[0], today); err != nil || day == "" {
				return fmt.Errorf("bad day %q", pos[0])
			}
		}
		tasks, err := s.All()
		if err != nil {
			return err
		}
		var todo, done []out
		for _, t := range tasks {
			p := store.Place(t, today)
			if p.Day != day {
				continue
			}
			o := view(s, t, p, false)
			if t.Done() {
				done = append(done, o)
			} else {
				todo = append(todo, o)
			}
		}
		if asJSON {
			return writeJSON(stdout, map[string]any{"day": day, "todo": nonNil(todo), "done": nonNil(done)})
		}
		fmt.Fprintf(stdout, "%s\n", strings.ToUpper(store.LongLabel(day)))
		printList(stdout, todo, today)
		if len(done) > 0 {
			fmt.Fprintln(stdout, "done:")
			printList(stdout, done, today)
		}
		if len(todo)+len(done) == 0 {
			fmt.Fprintln(stdout, "  nothing planned")
		}
		return nil

	case "list":
		tasks, err := s.All()
		if err != nil {
			return err
		}
		var list []out
		for _, t := range tasks {
			if p := flags["p"]; p != "" && t.Project != p {
				continue
			}
			if flags["all"] == "" && (flags["done"] != "") != t.Done() {
				continue
			}
			list = append(list, view(s, t, store.Place(t, today), false))
		}
		sortByDay(list)
		if asJSON {
			return writeJSON(stdout, nonNil(list))
		}
		printList(stdout, list, today)
		return nil

	case "show":
		t, err := one(s, pos)
		if err != nil {
			return err
		}
		o := view(s, t, store.Place(t, today), true)
		if asJSON {
			return writeJSON(stdout, o)
		}
		fmt.Fprintf(stdout, "%s  %s\n", o.ID, o.Title)
		fmt.Fprintf(stdout, "project: %s", o.Project)
		if o.Repo != "" {
			fmt.Fprintf(stdout, "  (%s)", o.Repo)
		}
		fmt.Fprintln(stdout)
		for _, kv := range [][2]string{{"platform", o.Platform}, {"status", o.Status}, {"url", o.URL}, {"planned", o.Planned}, {"carried from", o.CarriedFrom}, {"deadline", o.Deadline}, {"done", o.DoneAt}} {
			if kv[1] != "" {
				fmt.Fprintf(stdout, "%s: %s\n", kv[0], kv[1])
			}
		}
		fmt.Fprintf(stdout, "file: %s\n", o.File)
		if o.Description != "" {
			fmt.Fprintf(stdout, "\n%s\n", o.Description)
		}
		return nil

	case "add":
		if len(pos) == 0 {
			return errors.New("usage: tk add <title> -p <project>")
		}
		t := store.Task{Title: strings.Join(pos, " "), Project: flags["p"]}
		if err := applyEdits(&t, flags, stdin, today); err != nil {
			return err
		}
		t, err := s.Create(t)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "added %s  %s/%s\n", t.ID, t.Project, t.Title)
		return nil

	case "plan", "deadline":
		if len(pos) != 2 {
			return fmt.Errorf("usage: tk %s <id> <day|none>", cmd)
		}
		day, err := store.ParseDay(pos[1], today)
		if err != nil {
			return err
		}
		t, err := s.Update(pos[0], func(t *store.Task) error {
			if cmd == "plan" {
				t.Planned = day
			} else {
				t.Deadline = day
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s %s %s\n", t.ID, cmd, orNone(day))
		return nil

	case "done", "undo":
		if len(pos) != 1 {
			return errors.New("give exactly one task id")
		}
		// --on records a past publication or finish; the default is today.
		on := today
		if v, ok := flags["on"]; ok {
			if on, err = store.ParseDay(v, today); err != nil || on == "" {
				return fmt.Errorf("bad day %q", v)
			}
		}
		t, err := s.Update(pos[0], func(t *store.Task) error {
			if cmd == "done" {
				t.DoneAt = on
				if v := flags["url"]; v != "" {
					t.URL = v
				}
			} else {
				t.DoneAt = ""
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s %s  %s\n", t.ID, cmd, t.Title)
		return nil

	case "edit":
		if len(pos) != 1 {
			return errors.New("give exactly one task id")
		}
		// Read stdin before taking the lock; a slow pipe must not block the TUI.
		var edits store.Task
		if err := applyEdits(&edits, flags, stdin, today); err != nil {
			return err
		}
		t, err := s.Update(pos[0], func(t *store.Task) error {
			if v := flags["title"]; v != "" {
				t.Title = v
			}
			if v := flags["p"]; v != "" {
				t.Project = v
			}
			if _, ok := flags["plan"]; ok {
				t.Planned = edits.Planned
			}
			if _, ok := flags["deadline"]; ok {
				t.Deadline = edits.Deadline
			}
			if _, ok := flags["desc"]; ok {
				t.Body = edits.Body
			}
			if _, ok := flags["platform"]; ok {
				t.Platform = edits.Platform
			}
			if _, ok := flags["status"]; ok {
				t.Status = edits.Status
			}
			if _, ok := flags["url"]; ok {
				t.URL = edits.URL
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s saved\n", t.ID)
		return nil

	case "rm":
		if len(pos) != 1 {
			return errors.New("give exactly one task id")
		}
		t, err := s.Delete(pos[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s removed  %s\n", t.ID, t.Title)
		return nil

	case "projects":
		ps := s.Projects()
		if asJSON {
			return writeJSON(stdout, ps)
		}
		for _, p := range ps {
			fmt.Fprintf(stdout, "%-24s %s\n", p.Name, p.Path)
		}
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

// parse splits args into positionals and flags. Flags can sit anywhere so
// `tk add fix login -p acme-web` works. Unknown dash words stay in the
// title, so `tk add fix -v flag` keeps "-v".
func parse(args []string) ([]string, map[string]string, error) {
	bools := map[string]bool{"json": true, "done": true, "all": true}
	valued := map[string]bool{"p": true, "plan": true, "deadline": true, "desc": true, "title": true, "platform": true, "status": true, "url": true, "on": true}
	var pos []string
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-" || !strings.HasPrefix(a, "-") || isNumber(a) {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if name == "project" {
			name = "p"
		}
		if bools[name] {
			flags[name] = "1"
			continue
		}
		if !valued[name] {
			pos = append(pos, a)
			continue
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("flag %s needs a value", a)
		}
		flags[name] = args[i+1]
		i++
	}
	return pos, flags, nil
}

// "-1" is a day offset, not a flag.
func isNumber(a string) bool {
	return len(a) > 1 && strings.Trim(a[1:], "0123456789") == ""
}

func applyEdits(t *store.Task, flags map[string]string, stdin io.Reader, today string) error {
	if v, ok := flags["plan"]; ok {
		d, err := store.ParseDay(v, today)
		if err != nil {
			return err
		}
		t.Planned = d
	}
	if v, ok := flags["deadline"]; ok {
		d, err := store.ParseDay(v, today)
		if err != nil {
			return err
		}
		t.Deadline = d
	}
	if v, ok := flags["platform"]; ok {
		if v = strings.ToLower(v); v != "" && !platforms[v] {
			return fmt.Errorf("platform %q: use linkedin, instagram, facebook or tiktok", v)
		}
		t.Platform = v
	}
	if v, ok := flags["status"]; ok {
		if v = strings.ToLower(v); v != "" && !statuses[v] {
			return fmt.Errorf("status %q: use draft or approved; published is tk post done", v)
		}
		t.Status = v
	}
	if v, ok := flags["url"]; ok {
		t.URL = strings.TrimSpace(v)
	}
	if v, ok := flags["desc"]; ok {
		if v == "-" {
			b, err := io.ReadAll(stdin)
			if err != nil {
				return err
			}
			v = string(b)
			if strings.TrimSpace(v) == "" {
				return errors.New(`stdin was empty; use --desc "" to clear the description`)
			}
		}
		t.Body = v
	}
	return nil
}

func one(s *store.Store, pos []string) (store.Task, error) {
	if len(pos) != 1 {
		return store.Task{}, errors.New("give exactly one task id")
	}
	return s.Get(pos[0])
}

func view(s *store.Store, t store.Task, p store.Placement, withBody bool) out {
	o := out{
		ID: t.ID, Title: t.Title, Project: t.Project, Repo: s.RepoPath(t.Project),
		Platform: t.Platform, Status: t.Status, URL: t.URL,
		Planned: t.Planned, Deadline: t.Deadline, DoneAt: t.DoneAt,
		CarriedFrom: p.CarriedFrom, File: t.Path,
	}
	if withBody {
		o.Description = t.Body
	}
	return o
}

func printList(w io.Writer, list []out, today string) {
	for _, o := range list {
		box := "[ ]"
		if o.DoneAt != "" {
			box = "[x]"
		}
		var tags []string
		if o.CarriedFrom != "" {
			tags = append(tags, "carried from "+store.Label(o.CarriedFrom))
		} else if o.Planned != "" && o.DoneAt == "" {
			tags = append(tags, "planned "+store.Label(o.Planned))
		}
		if o.Deadline != "" && o.DoneAt == "" {
			mark := "deadline "
			if o.Deadline < today {
				mark = "OVERDUE "
			}
			tags = append(tags, mark+store.LongLabel(o.Deadline))
		}
		if o.DoneAt != "" {
			tags = append(tags, "done "+store.Label(o.DoneAt))
		}
		project := o.Project
		if o.Platform != "" {
			project += "/" + o.Platform
		}
		if o.Status != "" && o.DoneAt == "" {
			tags = append(tags, o.Status)
		}
		line := fmt.Sprintf("  %s %-7s %-16s %s", box, o.ID, project, o.Title)
		if len(tags) > 0 {
			line += "  (" + strings.Join(tags, ", ") + ")"
		}
		fmt.Fprintln(w, line)
	}
}

// sortByDay puts planned work first by date, then backlog, newest ids last.
func sortByDay(list []out) {
	key := func(o out) string {
		if o.DoneAt != "" {
			return "0" + o.DoneAt
		}
		if o.Planned != "" {
			return "1" + o.Planned
		}
		return "2"
	}
	sort.SliceStable(list, func(i, j int) bool { return key(list[i]) < key(list[j]) })
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func nonNil(l []out) []out {
	if l == nil {
		return []out{}
	}
	return l
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// Main is the entry point for non-TUI invocations.
func Main(args []string) {
	if err := Run(args, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "tk:", err)
		os.Exit(1)
	}
}
