package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hutchii/tk/internal/store"
)

func (m model) View() tea.View {
	if m.w < 24 || m.h < 8 {
		v := tea.NewView(ansi.Truncate("tk: window too small", max(m.w, 0), ""))
		v.AltScreen = true
		v.BackgroundColor = cBg
		return v
	}
	var body string
	switch {
	case m.mode == modeHelp:
		body = m.helpView()
	case m.mode == modeDetail || (m.mode == modePrompt && m.detailID != "" && m.backToDetail):
		body = m.vp.View()
	case m.view == viewCal:
		body = m.calView()
	default:
		body = m.boardView()
	}
	bh := m.h - 3
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.w, "")
	}
	body = lipgloss.NewStyle().Height(bh).MaxHeight(bh).Render(strings.Join(lines, "\n"))
	out := strings.Join([]string{m.headerBar(), body, m.statusLine(), m.hintLine()}, "\n")

	v := tea.NewView(out)
	v.AltScreen = true
	v.BackgroundColor = cBg
	v.ForegroundColor = cFg
	v.WindowTitle = "tk"
	return v
}

func (m model) headerBar() string {
	tab := func(n, label string, on bool) string {
		if on {
			return sTabOn.Render(n + " " + label)
		}
		return sTab.Render(n + " " + label)
	}
	left := sLogo.Render("tk") + " " + tab("1", "calendar", m.view == viewCal) + tab("2", "board", m.view == viewBoard)
	right := sDim.Render(store.LongLabel(m.today)) + " "
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, m.w, "")
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m model) statusLine() string {
	if m.mode == modePrompt {
		return sGreen.Render(" "+m.promptKind+" ▸ ") + m.input.View()
	}
	if m.status == "" {
		return ""
	}
	if m.isErr {
		return " " + sErr.Render(m.status)
	}
	return " " + sGreen.Render(m.status)
}

func (m model) hintLine() string {
	var h string
	switch {
	case m.mode == modePrompt:
		h = "enter save  esc cancel"
		if m.promptKind == "add" {
			h = "@project ^day !deadline  ·  enter save  esc cancel"
		}
	case m.mode == modeDetail:
		h = "e edit  x done  p plan  u deadline  r rename  m project  ↑↓ scroll  esc back"
	case m.view == viewCal:
		h = "a add  x done  p plan  [ ] move day  u deadline  e edit  ⏎ open  , . scroll  g today  2 board  ? keys  q quit"
	default:
		h = "a add  x done  p plan  d show done  e edit  ⏎ open  h/l projects/tasks  1 calendar  ? keys  q quit"
	}
	return sFaint.Render(ansi.Truncate(" "+h, m.w, "…"))
}

func (m model) helpView() string {
	rows := [][2]string{
		{"", "anywhere"},
		{"1 / 2 / tab", "calendar / board"},
		{"a", "add: title @project ^day !deadline"},
		{"x / space", "toggle done (done = today)"},
		{"p", "plan for a day: t m +3 fri 2026-10-12 none"},
		{"u", "set deadline"},
		{"r / m", "rename / move to project"},
		{"e", "edit description in $EDITOR (nvim)"},
		{"enter", "open task"},
		{"X", "delete task"},
		{"q", "quit"},
		{"", ""},
		{"", "calendar"},
		{"h j k l / arrows", "move"},
		{"[ ]", "move task a day back / forward (left of today = backlog)"},
		{", .", "scroll days"},
		{"g", "back to today"},
		{"", ""},
		{"", "board"},
		{"h / l", "projects / tasks"},
		{"d", "show or hide done"},
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, r := range rows {
		if r[0] == "" {
			b.WriteString("  " + sHead.Render(strings.ToUpper(r[1])) + "\n")
			continue
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", sGreen.Render(fmt.Sprintf("%-18s", r[0])), sBase.Render(r[1])))
	}
	b.WriteString("\n  " + sDim.Render("any key to close"))
	return b.String()
}

// ---- calendar

func (m model) calView() string {
	cols := m.columns()
	cw := m.colWidth()
	bh := m.h - 3
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = m.renderColumn(c, i, cw, bh)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m model) renderColumn(c column, ci, cw, bh int) string {
	inner := cw - 2
	var head string
	n := len(c.todo)
	switch {
	case c.day == "":
		head = sHead.Render(fmt.Sprintf("BACKLOG %d", n))
	case c.day == m.today:
		head = sToday.Render("TODAY "+store.Label(c.day)) + sDim.Render(fmt.Sprintf(" %d", n))
	case c.day < m.today:
		head = sDim.Render(store.Label(c.day))
	default:
		head = sHead.Render(store.Label(c.day)) + sDim.Render(fmt.Sprintf(" %d", n))
	}
	lines := []string{head, sFaint.Render(strings.Repeat("─", inner))}

	var body []string
	selStart, selEnd := -1, -1
	for ri, it := range c.items() {
		if ri == len(c.todo) && len(c.todo) > 0 {
			body = append(body, sFaint.Render(pad("── done ", inner, "─")))
		}
		selected := ci == m.col && ri == m.row && m.view == viewCal
		if selected {
			selStart = len(body)
		}
		body = append(body, m.calItem(it, inner, selected)...)
		if selected {
			selEnd = len(body)
		}
	}
	if len(body) == 0 && c.day != "" && c.day >= m.today {
		body = append(body, sFaint.Render("·"))
	}

	avail := bh - len(lines)
	off := 0
	if selEnd > avail {
		off = selEnd - avail
	}
	if selStart >= 0 && selStart < off {
		off = selStart
	}
	visible := body[off:]
	if len(visible) > avail {
		more := len(visible) - avail + 1
		visible = append(visible[:avail-1], sDim.Render(fmt.Sprintf("+%d lines", more)))
	}
	lines = append(lines, visible...)
	return lipgloss.NewStyle().Width(cw).PaddingRight(2).Render(strings.Join(lines, "\n"))
}

func (m model) calItem(it item, w int, selected bool) []string {
	t := it.t
	var flags []string
	var plainFlags []string
	if it.p.CarriedFrom != "" {
		flags = append(flags, sRed.Render("from "+store.Label(it.p.CarriedFrom)))
		plainFlags = append(plainFlags, "from "+store.Label(it.p.CarriedFrom))
	}
	if d := t.Deadline; d != "" && !t.Done() {
		s := "due " + shortDate(d)
		flags = append(flags, deadlineStyle(store.Deadline(t, m.today)).Render(s))
		plainFlags = append(plainFlags, s)
	}
	mark := "  "
	if t.Done() {
		mark = "✓ "
	}
	title := ansi.Truncate(mark+t.Title, w, "…")
	plain := strings.Join(plainFlags, " · ")
	styled := strings.Join(flags, sDim.Render(" · "))
	// Labels that don't fit beside the project get their own line between
	// project and title, so neither the project name nor the labels get cut.
	below := plain != "" && lipgloss.Width(t.Project)+1+lipgloss.Width(plain) > w

	// One line if both labels fit, else one line each.
	var plainBelow, styledBelow []string
	if below && lipgloss.Width(plain) <= w {
		plainBelow, styledBelow = []string{plain}, []string{styled}
	} else if below {
		for i := range flags {
			plainBelow = append(plainBelow, ansi.Truncate(plainFlags[i], w, "…"))
			styledBelow = append(styledBelow, ansi.Truncate(flags[i], w, "…"))
		}
	}

	if selected {
		if below {
			out := []string{sSelText.Width(w).Render(ansi.Truncate(t.Project, w, "…"))}
			for _, l := range plainBelow {
				out = append(out, sSelText.Width(w).Render(l))
			}
			return append(out, sSelText.Width(w).Render(title))
		}
		return []string{sSelText.Width(w).Render(spread(t.Project, plain, w)), sSelText.Width(w).Render(title)}
	}
	l2 := sBase.Render(title)
	if t.Done() {
		l2 = sGreen.Render("✓ ") + sDim.Render(ansi.Truncate(t.Title, w-2, "…"))
	}
	if below {
		out := append([]string{sDim.Render(ansi.Truncate(t.Project, w, "…"))}, styledBelow...)
		return append(out, l2)
	}
	return []string{spreadStyled(sDim.Render(t.Project), styled, w), l2}
}

// ---- board

func (m model) boardView() string {
	bh := m.h - 3
	sw := 24
	ps := m.projects()
	counts := map[string]int{}
	total := 0
	for _, t := range m.tasks {
		if !t.Done() {
			counts[t.Project]++
			total++
		}
	}
	side := []string{sHead.Render("PROJECTS"), sFaint.Render(strings.Repeat("─", sw-2))}
	for i, p := range ps {
		n := counts[p]
		if p == "all" {
			n = total
		}
		label := spread(p, fmt.Sprint(n), sw-2)
		switch {
		case i == m.proj && !m.focusList:
			side = append(side, sSelText.Width(sw-2).Render(label))
		case i == m.proj:
			side = append(side, sGreen.Render(label))
		default:
			side = append(side, sBase.Render(ansi.Truncate(label, sw-2, "…")))
		}
	}
	sideBox := lipgloss.NewStyle().Width(sw).Height(bh).MaxHeight(bh).Render(strings.Join(side, "\n"))

	lw := max(10, m.w-sw-1)
	todo, done := m.boardItems()
	name := m.boardProject()
	title := "all projects"
	sub := ""
	if name != "" {
		title = name
		sub = m.s.RepoPath(name)
	}
	lines := []string{sHead.Render(title) + "  " + sDim.Render(sub), sFaint.Render(strings.Repeat("─", lw-1))}
	lines = append(lines, sDim.Render(fmt.Sprintf("TO DO %d", len(todo))))
	var body []string
	selStart, selEnd := -1, -1
	row := 0
	addRow := func(it item) {
		sel := m.focusList && row == m.bRow
		if sel {
			selStart = len(body)
		}
		body = append(body, m.boardRow(it, lw-1, name == "", sel))
		if sel {
			selEnd = len(body)
		}
		row++
	}
	for _, it := range todo {
		addRow(it)
	}
	if len(todo) == 0 {
		body = append(body, sFaint.Render("  nothing to do"))
	}
	body = append(body, "")
	if m.showDone {
		body = append(body, sGreen.Render(fmt.Sprintf("▾ DONE %d", len(done))))
		for _, it := range done {
			addRow(it)
		}
	} else {
		body = append(body, sDim.Render(fmt.Sprintf("▸ DONE %d", len(done)))+sFaint.Render("  d to show"))
	}
	avail := bh - len(lines)
	off := 0
	if selEnd > avail {
		off = selEnd - avail
	}
	if selStart >= 0 && selStart < off {
		off = selStart
	}
	visible := body[off:]
	if len(visible) > avail {
		visible = visible[:avail]
	}
	list := strings.Join(append(lines, visible...), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, sideBox, " ", list)
}

func (m model) boardRow(it item, w int, showProject bool, selected bool) string {
	t := it.t
	mark := "  "
	if t.Done() {
		mark = "✓ "
	}
	var right []string
	if showProject {
		right = append(right, fmt.Sprintf("%-14s", ansi.Truncate(t.Project, 14, "…")))
	}
	when := ""
	switch {
	case t.Done():
		when = "done " + store.Label(t.DoneAt)
	case it.p.CarriedFrom != "":
		when = "from " + store.Label(it.p.CarriedFrom)
	case t.Planned != "":
		when = "⏵ " + store.Label(t.Planned)
	}
	right = append(right, fmt.Sprintf("%-12s", when))
	dl := ""
	if t.Deadline != "" && !t.Done() {
		dl = "due " + shortDate(t.Deadline)
	}
	right = append(right, fmt.Sprintf("%-10s", dl))
	r := strings.Join(right, " ")
	tw := max(8, w-lipgloss.Width(r)-1)
	left := fmt.Sprintf("%-*s", tw, ansi.Truncate(mark+t.Title, tw, "…"))

	if selected {
		return sSelText.Width(w).Render(left + " " + r)
	}
	ls := sBase.Render(left)
	if t.Done() {
		ls = sGreen.Render(mark) + sDim.Render(left[len(mark):])
	}
	whenStyle := sDim
	if it.p.CarriedFrom != "" {
		whenStyle = sRed
	}
	var rs []string
	if showProject {
		rs = append(rs, sDim.Render(right[0]))
	}
	rs = append(rs, whenStyle.Render(right[len(right)-2]), deadlineStyle(store.Deadline(t, m.today)).Render(right[len(right)-1]))
	return ls + " " + strings.Join(rs, " ")
}

// ---- detail

func (m *model) openDetail(id string) {
	t, err := m.s.Get(id)
	if err != nil {
		m.flash(err.Error(), true)
		m.mode = modeNormal
		return
	}
	m.mode = modeDetail
	m.detailID = id
	w := min(m.w-2, 100)

	var b strings.Builder
	b.WriteString("\n  " + sHead.Render(t.Title) + "\n\n")
	meta := func(k, v string, st lipgloss.Style) {
		if v != "" {
			b.WriteString("  " + sDim.Render(fmt.Sprintf("%-10s", k)) + st.Render(v) + "\n")
		}
	}
	meta("project", t.Project, sGreen)
	meta("repo", m.s.RepoPath(t.Project), sBase)
	p := store.Place(t, m.today)
	if t.Planned != "" {
		meta("planned", store.LongLabel(t.Planned), sBase)
	}
	if p.CarriedFrom != "" {
		meta("carried", "from "+store.LongLabel(p.CarriedFrom), sRed)
	}
	if t.Deadline != "" {
		meta("deadline", store.LongLabel(t.Deadline), deadlineStyle(store.Deadline(t, m.today)))
	}
	if t.Done() {
		meta("done", store.LongLabel(t.DoneAt), sGreen)
	}
	meta("id", t.ID, sDim)
	b.WriteString("  " + sFaint.Render(strings.Repeat("─", w-2)) + "\n")

	desc := strings.TrimSpace(t.Body)
	if desc == "" {
		b.WriteString("\n  " + sDim.Render("no description, press e to write one") + "\n")
	} else if r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(w-4)); err == nil {
		if out, err := r.Render(desc); err == nil {
			b.WriteString(out)
		} else {
			b.WriteString(desc)
		}
	}
	m.vp.SetWidth(m.w)
	m.vp.SetHeight(m.h - 3)
	m.vp.SetContent(b.String())
}

// ---- small helpers

func deadlineStyle(s store.DeadlineState) lipgloss.Style {
	switch s {
	case store.DeadlinePassed:
		return sRed
	case store.DeadlineSoon:
		return sYellow
	}
	return sDim
}

// shortDate renders 2026-10-10 as "10 oct".
func shortDate(d string) string {
	l := store.LongLabel(d)
	if i := strings.Index(l, " "); i >= 0 {
		return l[i+1:]
	}
	return l
}

func spread(left, right string, w int) string {
	if right == "" {
		return ansi.Truncate(left, w, "…")
	}
	left = ansi.Truncate(left, max(1, w-len([]rune(right))-1), "…")
	gap := max(1, w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func spreadStyled(left, right string, w int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > w {
		left = ansi.Truncate(left, max(1, w-rw-1), "…")
		lw = lipgloss.Width(left)
	}
	return left + strings.Repeat(" ", max(1, w-lw-rw)) + right
}

func pad(s string, w int, fill string) string {
	n := w - lipgloss.Width(s)
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(fill, n)
}
