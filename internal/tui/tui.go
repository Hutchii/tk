// Package tui is the full-screen app: a calendar of days and a per-project board.
package tui

import (
	"errors"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/hutchii/tk/internal/store"
)

type viewKind int

const (
	viewCal viewKind = iota
	viewBoard
)

type modeKind int

const (
	modeNormal modeKind = iota
	modePrompt
	modeDetail
	modeConfirm
	modeHelp
)

type item struct {
	t store.Task
	p store.Placement
}

type column struct {
	day  string // "" is the backlog
	todo []item
	done []item
}

func (c column) items() []item { return append(append([]item{}, c.todo...), c.done...) }

type model struct {
	s      *store.Store
	tasks  []store.Task
	today  string
	w, h   int
	view   viewKind
	mode   modeKind
	status string
	isErr  bool

	// calendar
	start    string // first visible day
	col, row int    // col 0 is the backlog
	selID    string // keeps the cursor on the same task across reloads and moves

	// board
	proj      int // 0 is "all"
	focusList bool
	showDone  bool
	bRow      int

	input        textinput.Model
	promptKind   string
	vp           viewport.Model
	detailID     string
	backToDetail bool // a prompt opened from the detail view returns there
}

type tickMsg struct{}
type editorDoneMsg struct {
	id, path, orig string
	err            error
}

func Run() error {
	s, err := store.Open()
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(newModel(s)).Run()
	return err
}

func newModel(s *store.Store) model {
	in := textinput.New()
	in.Prompt = ""
	m := model{s: s, input: in, w: 100, h: 30, vp: viewport.New()}
	m.reload()
	m.start = m.today
	m.col = 1
	m.syncSel()
	return m
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Init() tea.Cmd { return tick() }

// reload re-reads every task file. Claude or an editor may change them while
// the app is open, and a few hundred small files read in well under a frame.
func (m *model) reload() {
	tasks, err := m.s.All()
	m.tasks = tasks
	prev := m.today
	m.today = store.Today()
	if prev != "" && prev != m.today && m.start == prev {
		m.start = m.today // past midnight, keep today in view
	}
	if err != nil {
		m.flash(err.Error(), true)
	}
}

func (m *model) flash(s string, isErr bool) { m.status, m.isErr = s, isErr }

// ---- layout helpers

func (m model) dayCount() int {
	n := m.w/26 - 1
	return max(1, min(7, n))
}

func (m model) colWidth() int { return m.w / (m.dayCount() + 1) }

func (m model) columns() []column {
	n := m.dayCount()
	cols := make([]column, n+1)
	idx := map[string]int{}
	for i := 1; i <= n; i++ {
		cols[i].day = store.AddDays(m.start, i-1)
		idx[cols[i].day] = i
	}
	for _, t := range m.tasks {
		p := store.Place(t, m.today)
		ci, ok := 0, true
		if p.Day != "" {
			ci, ok = idx[p.Day]
		} else if t.Done() {
			ok = false
		}
		if !ok {
			continue
		}
		it := item{t, p}
		if t.Done() {
			cols[ci].done = append(cols[ci].done, it)
		} else {
			cols[ci].todo = append(cols[ci].todo, it)
		}
	}
	for i := range cols {
		sortTodo(cols[i].todo)
		sort.SliceStable(cols[i].done, func(a, b int) bool { return less(cols[i].done[a].t, cols[i].done[b].t) })
	}
	return cols
}

// Carried-over first, then nearest deadline, then project.
func sortTodo(l []item) {
	sort.SliceStable(l, func(a, b int) bool {
		x, y := l[a], l[b]
		if (x.p.CarriedFrom != "") != (y.p.CarriedFrom != "") {
			return x.p.CarriedFrom != ""
		}
		dx, dy := x.t.Deadline, y.t.Deadline
		if dx != dy {
			if dx == "" || dy == "" {
				return dy == ""
			}
			return dx < dy
		}
		return less(x.t, y.t)
	})
}

func less(a, b store.Task) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}
	return a.ID < b.ID
}

func (m model) projects() []string {
	names := []string{"all"}
	for _, p := range m.s.Projects() {
		names = append(names, p.Name)
	}
	return names
}

func (m model) boardProject() string {
	ps := m.projects()
	if m.proj <= 0 || m.proj >= len(ps) {
		return ""
	}
	return ps[m.proj]
}

func (m model) boardItems() (todo, done []item) {
	p := m.boardProject()
	for _, t := range m.tasks {
		if p != "" && t.Project != p {
			continue
		}
		it := item{t, store.Place(t, m.today)}
		if t.Done() {
			done = append(done, it)
		} else {
			todo = append(todo, it)
		}
	}
	sort.SliceStable(todo, func(a, b int) bool {
		x, y := todo[a].t, todo[b].t
		if x.Planned != y.Planned {
			if x.Planned == "" || y.Planned == "" {
				return y.Planned == ""
			}
			return x.Planned < y.Planned
		}
		return less(x, y)
	})
	sort.SliceStable(done, func(a, b int) bool { return done[a].t.DoneAt > done[b].t.DoneAt })
	return todo, done
}

func (m model) boardList() []item {
	todo, done := m.boardItems()
	if m.showDone {
		return append(todo, done...)
	}
	return todo
}

func (m model) current() (store.Task, bool) {
	if m.view == viewCal {
		cols := m.columns()
		if m.col < len(cols) {
			if its := cols[m.col].items(); m.row < len(its) {
				return its[m.row].t, true
			}
		}
		return store.Task{}, false
	}
	if !m.focusList {
		return store.Task{}, false
	}
	if l := m.boardList(); m.bRow < len(l) {
		return l[m.bRow].t, true
	}
	return store.Task{}, false
}

func (m *model) clamp() {
	n := m.dayCount()
	m.col = max(0, min(n, m.col))
	if m.view == viewCal {
		its := m.columns()[m.col].items()
		m.row = max(0, min(len(its)-1, m.row))
	} else {
		m.proj = max(0, min(len(m.projects())-1, m.proj))
		m.bRow = max(0, min(len(m.boardList())-1, m.bRow))
	}
}

func (m *model) syncSel() {
	m.clamp()
	if t, ok := m.current(); ok {
		m.selID = t.ID
	}
}

// follow puts the cursor on task id. With scroll it also moves the visible
// days so the task's column is on screen.
func (m *model) follow(id string, scroll bool) {
	if id == "" {
		m.clamp()
		return
	}
	if m.view == viewBoard {
		for i, it := range m.boardList() {
			if it.t.ID == id {
				m.bRow = i
				return
			}
		}
		m.clamp()
		return
	}
	if scroll {
		for _, t := range m.tasks {
			if t.ID != id {
				continue
			}
			day := store.Place(t, m.today).Day
			last := store.AddDays(m.start, m.dayCount()-1)
			if day != "" && day < m.start {
				m.start = day
			} else if day > last {
				m.start = store.AddDays(day, -(m.dayCount() - 1))
			}
		}
	}
	for ci, c := range m.columns() {
		for ri, it := range c.items() {
			if it.t.ID == id {
				m.col, m.row = ci, ri
				return
			}
		}
	}
	m.clamp()
}

// ---- update

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.input.SetWidth(max(10, m.w-30))
		if m.mode == modeDetail {
			m.openDetail(m.detailID)
		}
		m.follow(m.selID, false)
		return m, nil

	case tickMsg:
		m.reload()
		m.follow(m.selID, false)
		return m, tick()

	case editorDoneMsg:
		m.editorDone(msg)
		if m.mode == modeDetail {
			m.openDetail(msg.id)
		}
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modePrompt:
			switch key {
			case "esc", "enter":
				m.mode = modeNormal
				m.flash("", false)
				if key == "enter" {
					m.submitPrompt(strings.TrimSpace(m.input.Value()))
				}
				if m.backToDetail {
					m.backToDetail = false
					m.openDetail(m.detailID)
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case modeConfirm:
			m.mode = modeNormal
			if key == "y" {
				if t, err := m.s.Delete(m.selID); err != nil {
					m.flash(err.Error(), true)
				} else {
					m.flash("deleted "+t.Title, false)
				}
				m.reload()
				m.syncSel()
			} else {
				m.flash("", false)
			}
			return m, nil
		case modeHelp:
			m.mode = modeNormal
			return m, nil
		case modeDetail:
			return m.updateDetail(msg)
		}
		return m.updateNormal(key)
	}
	return m, nil
}

func (m model) updateNormal(key string) (tea.Model, tea.Cmd) {
	m.flash("", false)
	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
		return m, nil
	case "1":
		m.view = viewCal
		m.follow(m.selID, true)
		return m, nil
	case "2":
		m.view = viewBoard
		m.focusList = true
		m.follow(m.selID, false)
		return m, nil
	case "tab":
		if m.view == viewCal {
			return m.updateNormal("2")
		}
		return m.updateNormal("1")
	case "a":
		m.prompt("add", "")
		return m, m.input.Focus()
	}

	if m.view == viewCal {
		switch key {
		case "h", "left":
			if m.col > 0 {
				m.col--
			}
		case "l", "right":
			if m.col < m.dayCount() {
				m.col++
			} else {
				m.start = store.AddDays(m.start, 1)
			}
		case "j", "down":
			m.row++
		case "k", "up":
			m.row--
		case ",":
			m.start = store.AddDays(m.start, -m.dayCount())
		case ".":
			m.start = store.AddDays(m.start, m.dayCount())
		case "g":
			m.start, m.col = m.today, 1
		case "[", "]":
			m.moveDay(key == "]")
			return m, nil
		default:
			return m.taskKey(key)
		}
		m.syncSel()
		return m, nil
	}

	if !m.focusList {
		switch key {
		case "j", "down":
			m.proj++
		case "k", "up":
			m.proj--
		case "l", "right", "enter":
			m.focusList = true
			m.bRow = 0
		case "d":
			m.showDone = !m.showDone
		}
		m.syncSel()
		return m, nil
	}
	switch key {
	case "h", "left", "esc":
		m.focusList = false
	case "j", "down":
		m.bRow++
	case "k", "up":
		m.bRow--
	case "d":
		m.showDone = !m.showDone
	default:
		return m.taskKey(key)
	}
	m.syncSel()
	return m, nil
}

// taskKey handles keys that act on the selected task in either view.
func (m model) taskKey(key string) (tea.Model, tea.Cmd) {
	t, ok := m.current()
	if !ok {
		return m, nil
	}
	m.selID = t.ID
	switch key {
	case "x", "space":
		m.toggleDone(t.ID)
		m.follow(t.ID, true)
	case "p":
		m.prompt("plan", t.Planned)
		return m, m.input.Focus()
	case "u":
		m.prompt("deadline", t.Deadline)
		return m, m.input.Focus()
	case "r":
		m.prompt("rename", t.Title)
		return m, m.input.Focus()
	case "m":
		m.prompt("project", t.Project)
		return m, m.input.Focus()
	case "X":
		m.mode = modeConfirm
		m.flash("delete \""+t.Title+"\"? y/n", true)
	case "e":
		return m, m.editCmd(t.ID)
	case "enter":
		m.openDetail(t.ID)
	}
	return m, nil
}

// moveDay shifts a task one day. Left of today is the backlog; done tasks
// stay on the day they were done.
func (m *model) moveDay(forward bool) {
	cur, ok := m.current()
	if !ok {
		return
	}
	errDone := errors.New("done tasks stay on the day they were done (x to undo)")
	m.update(cur.ID, "", func(t *store.Task) error {
		if t.Done() {
			return errDone
		}
		day := store.Place(*t, m.today).Day
		switch {
		case forward && day == "":
			t.Planned = m.today
		case forward:
			t.Planned = store.AddDays(day, 1)
		case day == "":
		default:
			prev := store.AddDays(day, -1)
			if prev < m.today {
				prev = ""
			}
			t.Planned = prev
		}
		return nil
	})
	m.follow(cur.ID, true)
}

func (m *model) toggleDone(id string) {
	t, ok := m.update(id, "", func(t *store.Task) error {
		if t.Done() {
			t.DoneAt = ""
		} else {
			t.DoneAt = m.today
		}
		return nil
	})
	if ok && t.Done() {
		m.flash("done: "+t.Title, false)
	} else if ok {
		m.flash("not done: "+t.Title, false)
	}
}

// update is the only way the app changes a task. store.Update re-reads the
// file under the lock, because m.tasks can be a tick old and saving that copy
// would undo an edit Claude made through the CLI.
func (m *model) update(id, note string, fn func(*store.Task) error) (store.Task, bool) {
	t, err := m.s.Update(id, fn)
	m.reload()
	if err != nil {
		m.flash(err.Error(), true)
		return t, false
	}
	if note != "" {
		m.flash(note, false)
	}
	return t, true
}

// ---- prompts

func (m *model) prompt(kind, value string) {
	m.mode = modePrompt
	m.promptKind = kind
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Placeholder = map[string]string{
		"add":      "title @project ^day !deadline",
		"plan":     "t, m, +3, fri, 2026-10-12, none",
		"deadline": "t, m, +3, fri, 2026-10-12, none",
		"rename":   "new title",
		"project":  "project name",
	}[kind]
}

func (m *model) submitPrompt(v string) {
	if m.promptKind == "add" {
		m.addFromPrompt(v)
		return
	}
	kind := m.promptKind
	day := ""
	switch kind {
	case "plan", "deadline":
		var err error
		if day, err = store.ParseDay(v, m.today); err != nil {
			m.flash(err.Error(), true)
			return
		}
	case "rename", "project":
		if v == "" {
			return
		}
	}
	m.update(m.selID, kind+" saved", func(t *store.Task) error {
		switch kind {
		case "plan":
			t.Planned = day
		case "deadline":
			t.Deadline = day
		case "rename":
			t.Title = v
		case "project":
			t.Project = v
		}
		return nil
	})
	m.follow(m.selID, true)
}

// addFromPrompt reads "title @project ^day !deadline". Project defaults to
// the selected board project or the task under the cursor; day defaults to
// the calendar column the cursor is on.
func (m *model) addFromPrompt(v string) {
	var title []string
	project, plan, deadline := "", "", ""
	hasPlan := false
	for _, w := range strings.Fields(v) {
		switch {
		case len(w) > 1 && w[0] == '@':
			project = w[1:]
		case len(w) > 1 && w[0] == '^':
			plan, hasPlan = w[1:], true
		case len(w) > 1 && w[0] == '!':
			deadline = w[1:]
		default:
			title = append(title, w)
		}
	}
	if project == "" {
		project = m.boardProject()
	}
	if project == "" {
		if t, ok := m.current(); ok {
			project = t.Project
		}
	}
	if project == "" {
		m.flash("which project? add @name", true)
		return
	}
	day := ""
	if hasPlan {
		var err error
		if day, err = store.ParseDay(plan, m.today); err != nil {
			m.flash(err.Error(), true)
			return
		}
	} else if m.view == viewCal && m.col > 0 {
		if d := m.columns()[m.col].day; d >= m.today {
			day = d
		}
	}
	dl, err := store.ParseDay(deadline, m.today)
	if err != nil {
		m.flash(err.Error(), true)
		return
	}
	t, err := m.s.Create(store.Task{Title: strings.Join(title, " "), Project: project, Planned: day, Deadline: dl})
	m.reload()
	if err != nil {
		m.flash(err.Error(), true)
		return
	}
	m.flash("added "+t.Title, false)
	m.follow(t.ID, true)
}

// ---- detail + editor

func (m model) updateDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.mode = modeNormal
		m.follow(m.detailID, true)
		return m, nil
	case "e", "x", "p", "u", "r", "m":
		t, err := m.s.Get(m.detailID)
		if err != nil {
			m.flash(err.Error(), true)
			return m, nil
		}
		m.selID = t.ID
		if msg.String() == "e" {
			return m, m.editCmd(t.ID)
		}
		if msg.String() == "x" {
			m.toggleDone(t.ID)
			m.openDetail(t.ID)
			return m, nil
		}
		value := map[string]string{"p": t.Planned, "u": t.Deadline, "r": t.Title, "m": t.Project}[msg.String()]
		kind := map[string]string{"p": "plan", "u": "deadline", "r": "rename", "m": "project"}[msg.String()]
		m.prompt(kind, value)
		m.backToDetail = true
		return m, m.input.Focus()
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m *model) editCmd(id string) tea.Cmd {
	t, err := m.s.Get(id)
	if err != nil {
		m.flash(err.Error(), true)
		return nil
	}
	f, err := os.CreateTemp("", "tk-"+t.ID+"-*.md")
	if err != nil {
		m.flash(err.Error(), true)
		return nil
	}
	f.WriteString(t.Body)
	f.Close()
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
		if _, err := exec.LookPath("nvim"); err == nil {
			editor = "nvim"
		}
	}
	parts := strings.Fields(editor) // allows "code --wait"
	c := exec.Command(parts[0], append(parts[1:], f.Name())...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorDoneMsg{id: t.ID, path: f.Name(), orig: t.Body, err: err}
	})
}

// editorDone saves the edited description. The temp file is kept whenever the
// text is not saved, so nothing typed is lost: on an editor error, and when
// the description changed on disk while the editor was open.
func (m *model) editorDone(msg editorDoneMsg) {
	if msg.err != nil {
		m.flash("editor exited with "+msg.err.Error()+"; your text is in "+msg.path, true)
		return
	}
	raw, err := os.ReadFile(msg.path)
	if err != nil {
		m.flash(err.Error(), true)
		return
	}
	body := string(raw)
	if body == msg.orig {
		os.Remove(msg.path)
		return
	}
	errConflict := errors.New("description changed while you were editing; your text is in " + msg.path)
	if _, ok := m.update(msg.id, "description saved", func(t *store.Task) error {
		if t.Body != msg.orig {
			return errConflict
		}
		t.Body = body
		return nil
	}); ok {
		os.Remove(msg.path)
	}
}
