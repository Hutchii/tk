package tui

import "charm.land/lipgloss/v2"

// One palette, dark only. Green means done or selected, red means late,
// yellow means a deadline is close. Everything else is grey or off-white.
var (
	cBg     = lipgloss.Color("#0a0a0a")
	cFg     = lipgloss.Color("#c8c8c8")
	cDim    = lipgloss.Color("#5f5f5f")
	cFaint  = lipgloss.Color("#3a3a3a")
	cGreen  = lipgloss.Color("#3ddc84")
	cRed    = lipgloss.Color("#ff4747")
	cYellow = lipgloss.Color("#f0c040")
	cSelBg  = lipgloss.Color("#16301f")
	cMuted  = lipgloss.Color("#8a8a8a")

	sBase    = lipgloss.NewStyle().Foreground(cFg)
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sFaint   = lipgloss.NewStyle().Foreground(cFaint)
	sGreen   = lipgloss.NewStyle().Foreground(cGreen)
	sRed     = lipgloss.NewStyle().Foreground(cRed)
	sYellow  = lipgloss.NewStyle().Foreground(cYellow)
	sHead    = lipgloss.NewStyle().Foreground(cFg).Bold(true)
	sToday   = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	sLogo    = lipgloss.NewStyle().Foreground(cBg).Background(cGreen).Bold(true).Padding(0, 1)
	sTab     = lipgloss.NewStyle().Foreground(cDim).Padding(0, 1)
	sTabOn   = lipgloss.NewStyle().Foreground(cGreen).Bold(true).Padding(0, 1)
	sSel     = lipgloss.NewStyle().Background(cSelBg)
	sSelText = lipgloss.NewStyle().Background(cSelBg).Foreground(cGreen).Bold(true)
	sErr     = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sDesc    = lipgloss.NewStyle().Foreground(cMuted)
	sSelDesc = lipgloss.NewStyle().Background(cSelBg).Foreground(cMuted)
)

// Post platforms get their brand colour so a column of posts reads at a glance.
var platformStyle = map[string]lipgloss.Style{
	"linkedin":  lipgloss.NewStyle().Background(lipgloss.Color("#0a66c2")).Foreground(lipgloss.Color("#ffffff")),
	"facebook":  lipgloss.NewStyle().Background(lipgloss.Color("#4267b2")).Foreground(lipgloss.Color("#ffffff")),
	"instagram": lipgloss.NewStyle().Background(lipgloss.Color("#e1306c")).Foreground(lipgloss.Color("#ffffff")),
	"tiktok":    lipgloss.NewStyle().Background(lipgloss.Color("#25f4ee")).Foreground(lipgloss.Color("#000000")),
}
