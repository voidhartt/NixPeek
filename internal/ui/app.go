package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"nixpeek/internal/actions"
	"nixpeek/internal/clipboard"
	"nixpeek/internal/models"
	"nixpeek/internal/search"
)

type status string

const (
	statusTyping    status = "typing"
	statusSearching status = "searching"
	statusLoaded    status = "loaded"
	statusErr       status = "error"
)

var (
	screenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	containerStyle = lipgloss.NewStyle().
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Padding(0, 1)

	headerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255"))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("254"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246"))

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	okStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("114"))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))
)

type App struct {
	service       *search.Service
	input         textinput.Model
	inputFocused  bool
	query         string
	pkgs          []models.Package
	cursor        int
	showDetails   bool
	showHelp      bool
	status        status
	err           string
	filters       models.SearchFilters
	copied        string
	copyErr       string
	lastRequestID int
	cancel        context.CancelFunc
	width         int
	height        int
}

type searchMsg struct {
	id   int
	pkgs []models.Package
	err  error
}

type debounceMsg struct{ id int }

type shortcutEntry struct {
	key    string
	action string
}

func New(service *search.Service, initialQuery string) *App {
	in := textinput.New()
	in.Placeholder = "type here"
	in.Focus()
	in.SetValue(initialQuery)
	in.CharLimit = 256
	in.Width = 60
	in.Prompt = "> "

	return &App{
		service:      service,
		input:        in,
		inputFocused: true,
		query:        initialQuery,
		status:       statusTyping,
		showDetails:  true,
		filters:      models.SearchFilters{Scope: models.ScopeNameDescription, MatchMode: models.MatchContains},
	}
}

func (a *App) Init() tea.Cmd {
	if strings.TrimSpace(a.query) == "" {
		return textinput.Blink
	}
	a.lastRequestID++
	id := a.lastRequestID
	return tea.Batch(textinput.Blink, a.debounceCmd(id))
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizeInput()
		return a, nil
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c", "alt+q":
			if a.cancel != nil {
				a.cancel()
			}
			return a, tea.Quit
		case "?":
			a.showHelp = !a.showHelp
			return a, nil
		case "tab":
			a.showDetails = !a.showDetails
			return a, nil
		case "enter":
			a.inputFocused = true
			a.input.Focus()
			return a, nil
		case "esc":
			a.inputFocused = false
			a.input.Blur()
			return a, nil
		}

		if a.handleActionKey(key) {
			return a, nil
		}

		if !a.inputFocused {
			switch key {
			case "c":
				if p, ok := a.selected(); ok {
					a.handleCopy(actions.AttrPath(p.AttrPath))
				}
				return a, nil
			case "i":
				if p, ok := a.selected(); ok {
					a.handleCopy(actions.NixProfileInstall(p.AttrPath))
				}
				return a, nil
			case "r":
				if p, ok := a.selected(); ok {
					a.handleCopy(actions.NixRun(p.AttrPath))
				}
				return a, nil
			}
		}

		switch key {
		case "up", "k":
			if a.cursor > 0 {
				a.cursor--
			}
			return a, nil
		case "down", "j":
			if a.cursor < len(a.pkgs)-1 {
				a.cursor++
			}
			return a, nil
		case "ctrl+n":
			a.filters.Scope = models.ScopeNameOnly
			return a, a.triggerSearch()
		case "ctrl+d":
			a.filters.Scope = models.ScopeNameDescription
			return a, a.triggerSearch()
		case "ctrl+p":
			a.filters.MatchMode = models.MatchPrefix
			return a, a.triggerSearch()
		case "ctrl+o":
			a.filters.MatchMode = models.MatchContains
			return a, a.triggerSearch()
		case "ctrl+e":
			a.filters.ExactAttr = !a.filters.ExactAttr
			return a, a.triggerSearch()
		}

		if !a.inputFocused {
			return a, nil
		}

		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		newQ := a.input.Value()
		if newQ != a.query {
			a.query = newQ
			a.status = statusTyping
			a.err = ""
			a.lastRequestID++
			id := a.lastRequestID
			return a, tea.Batch(cmd, a.debounceCmd(id))
		}
		return a, cmd
	case debounceMsg:
		if msg.id != a.lastRequestID {
			return a, nil
		}
		return a, a.runSearch(msg.id)
	case searchMsg:
		if msg.id != a.lastRequestID {
			return a, nil
		}
		if msg.err != nil {
			a.status = statusErr
			a.err = msg.err.Error()
			a.pkgs = nil
			a.cursor = 0
			return a, nil
		}
		a.status = statusLoaded
		a.err = ""
		a.pkgs = msg.pkgs
		a.cursor = 0
		return a, nil
	}
	return a, nil
}

func (a *App) View() string {
	if a.width <= 0 || a.height <= 0 {
		return "Loading NixPeek..."
	}

	contentW := a.contentWidth()
	contentH := a.contentHeight()
	if a.input.Width != max(12, contentW-14) {
		a.input.Width = max(12, contentW-14)
	}

	if a.showHelp {
		help := a.renderHelpPanel(contentW, contentH)
		return a.wrapScreen(help, contentW, contentH)
	}

	header := a.renderHeader(contentW)
	searchPanel := a.renderSearchPanel(contentW, 5)
	footer := a.renderFooter(contentW)

	bodyH := contentH - lipgloss.Height(header) - lipgloss.Height(searchPanel)
	if footer != "" {
		bodyH -= lipgloss.Height(footer)
	}
	if bodyH < 6 {
		bodyH = 6
	}
	body := a.renderBody(contentW, bodyH)

	parts := []string{header, searchPanel, body}
	if footer != "" {
		parts = append(parts, footer)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return a.wrapScreen(content, contentW, contentH)
}

func (a *App) renderHeader(width int) string {
	title := trim(" NixPeek", max(1, width-2))
	return headerStyle.Width(width).Render(headerTitleStyle.Render(title))
}

func (a *App) renderSearchPanel(width, height int) string {
	queryLine := a.input.View()
	if !a.inputFocused {
		queryLine = dimStyle.Render(queryLine + "  (press Enter to focus search)")
	}

	meta := ""
	if a.err != "" {
		meta = errStyle.Render("error: " + trim(a.err, max(12, width-16)))
	}
	lines := []string{queryLine}
	if meta != "" {
		lines = append(lines, dimStyle.Render(meta))
	}
	return renderPanel("", width, height, lines)
}

func (a *App) renderBody(width, height int) string {
	if !a.showDetails {
		return a.renderListPanel(width, height)
	}

	if width >= 120 {
		gap := 1
		leftW := clamp(int(float64(width)*0.42), 34, width-40)
		rightW := width - leftW - gap
		left := a.renderListPanel(leftW, height)
		shortH := clamp(height/5, 7, 9)
		detailsH := height - shortH
		if detailsH < 5 {
			detailsH = max(1, height-4)
			shortH = max(1, height-detailsH)
		}
		details := a.renderDetailsPanel(rightW, detailsH)
		shortcuts := a.renderShortcutsPanel(rightW, shortH)
		right := lipgloss.JoinVertical(lipgloss.Left, details, shortcuts)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
	}

	listH := clamp(height/2, 7, height-7)
	remaining := height - listH
	gap := 1
	shortH := clamp(remaining/4, 7, 9)
	detailsH := remaining - shortH - gap
	if detailsH < 7 {
		detailsH = max(6, remaining-gap-4)
		shortH = max(4, remaining-detailsH-gap)
	}
	top := a.renderListPanel(width, listH)
	details := a.renderDetailsPanel(width, detailsH)
	shortcuts := a.renderShortcutsPanel(width, shortH)
	return lipgloss.JoinVertical(lipgloss.Left, top, details, shortcuts)
}

func (a *App) renderListPanel(width, height int) string {
	visible := max(1, panelBodyCapacity(height)-1)
	innerW := max(8, width-panelStyle.GetHorizontalFrameSize())
	lines := a.renderListLines(visible, innerW-2)
	return renderPanel(fmt.Sprintf("Packages (%d)", len(a.pkgs)), width, height, lines)
}

func (a *App) renderDetailsPanel(width, height int) string {
	if len(a.pkgs) == 0 {
		return renderPanel("Details", width, height, []string{"No package selected"})
	}

	p, _ := a.selected()
	rows := [][2]string{
		{"AttrPath", p.AttrPath},
		{"Name", firstNonEmpty(p.PName, p.Name)},
		{"Version", emptyDash(p.Version)},
		{"Installed", installedLabel(p.Installed)},
		{"Description", emptyDash(p.Description)},
		{"Homepage", emptyDash(p.Homepage)},
		{"License", emptyDash(p.License)},
		{"Platforms", emptyDash(strings.Join(p.Platforms, ", "))},
		{"Install", actions.NixProfileInstall(p.AttrPath)},
		{"Run", actions.NixRun(p.AttrPath) + " (not all packages are runnable)"},
	}

	bodyWidth := max(8, width-panelStyle.GetHorizontalFrameSize()-2)
	lines := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		lines = append(lines, renderRow(row[0], row[1], bodyWidth)...)
	}
	return renderPanel("Details", width, height, lines)
}

func (a *App) renderShortcutsPanel(width, height int) string {
	lines := a.shortcutLines(width)
	return renderPanel("Shortcuts", width, height, lines)
}

func (a *App) renderFooter(width int) string {
	if a.copyErr != "" {
		return errStyle.Width(width).Render("copy failed: " + trim(a.copyErr, max(10, width-14)))
	}
	if a.copied != "" {
		return okStyle.Width(width).Render("copied: " + trim(a.copied, max(10, width-9)))
	}
	return ""
}

func (a *App) renderHelpPanel(width, height int) string {
	lines := []string{
		"NixPeek Keybindings",
		"",
		"enter           focus search input",
		"esc             focus package list",
		"up/down, j/k    move selection",
		"ctrl+n          scope name only",
		"ctrl+d          scope name + description",
		"ctrl+p          match prefix",
		"ctrl+o          match contains",
		"ctrl+e          toggle exact attrPath",
		"alt+c           copy attrPath",
		"alt+i           copy nix profile command",
		"alt+r           copy nix run command",
		"c / i / r       same actions in list mode",
		"tab             toggle details panel",
		"?               close/open this help",
		"alt+q / ctrl+c  quit",
	}
	return renderPanel("Help", width, height, lines)
}

func (a *App) shortcutLines(width int) []string {
	inner := max(24, width-panelStyle.GetHorizontalFrameSize())
	rows := [][2]shortcutEntry{
		{{key: "Alt+q", action: "quit"}, {key: "Ctrl+c", action: "force quit"}},
		{{key: "Enter", action: "focus search"}, {key: "Esc", action: "focus list"}},
		{{key: "j/k", action: "move"}, {key: "Tab", action: "details"}},
		{{key: "?", action: "help"}, {key: "Alt+c/i/r or c/i/r", action: "copy"}},
	}

	if inner >= 66 {
		gridW := min(inner, 84)
		gap := 3
		leftEntries := make([]shortcutEntry, 0, len(rows))
		rightEntries := make([]shortcutEntry, 0, len(rows))
		for _, row := range rows {
			leftEntries = append(leftEntries, row[0])
			rightEntries = append(rightEntries, row[1])
		}
		leftKeyW := shortcutMaxKeyWidth(leftEntries)
		rightKeyW := shortcutMaxKeyWidth(rightEntries)
		lines := make([]string, 0, len(rows))
		for _, row := range rows {
			left := formatShortcutEntry(row[0].key, row[0].action, leftKeyW)
			right := formatShortcutEntry(row[1].key, row[1].action, rightKeyW)
			lines = append(lines, shortcutTwoColLeftLine(left, right, gridW, gap))
		}
		return lines
	}

	flat := make([]shortcutEntry, 0, len(rows)*2)
	for _, row := range rows {
		flat = append(flat, row[0], row[1])
	}
	keyW := shortcutMaxKeyWidth(flat)
	lines := make([]string, 0, len(flat))
	for _, entry := range flat {
		lines = append(lines, formatShortcutEntry(entry.key, entry.action, keyW))
	}
	return lines
}

func renderPanel(title string, width, height int, bodyLines []string) string {
	width = max(20, width)
	height = max(4, height)

	frameW := panelStyle.GetHorizontalFrameSize()
	frameH := panelStyle.GetVerticalFrameSize()
	innerW := max(1, width-frameW)
	innerH := max(1, height-frameH)

	capacity := innerH
	titleLine := ""
	if strings.TrimSpace(title) != "" {
		capacity = innerH - 1 // one line for panel title
		titleLine = panelTitleStyle.Render(title)
	}
	if capacity < 1 {
		capacity = 1
	}
	lines := fitLines(bodyLines, capacity)
	for len(lines) < capacity {
		lines = append(lines, "")
	}

	content := lines
	if titleLine != "" {
		content = append([]string{titleLine}, lines...)
	}
	return panelStyle.Width(innerW).Height(innerH).Render(strings.Join(content, "\n"))
}

func panelBodyCapacity(panelHeight int) int {
	return max(1, panelHeight-panelStyle.GetVerticalFrameSize())
}

func (a *App) renderListLines(visible, width int) []string {
	if len(a.pkgs) == 0 {
		if strings.TrimSpace(a.query) == "" {
			return []string{dimStyle.Render("Start typing package name or attrPath...")}
		}
		if a.status == statusSearching || a.status == statusTyping {
			return []string{dimStyle.Render("Searching...")}
		}
		return []string{dimStyle.Render("No matches")}
	}

	start := 0
	if a.cursor >= visible {
		start = a.cursor - visible + 1
	}
	if maxStart := len(a.pkgs) - visible; maxStart > 0 && start > maxStart {
		start = maxStart
	}

	end := min(len(a.pkgs), start+visible)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		p := a.pkgs[i]
		marker := "  "
		if i == a.cursor {
			marker = "▸ "
		}
		inst := ""
		if p.Installed {
			inst = " " + okStyle.Render("[installed]")
		}
		nameWidth := max(8, width-16)
		line := fmt.Sprintf("%s%s%s", marker, trim(p.AttrPath, nameWidth), inst)
		if i == a.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return lines
}

func renderRow(key, value string, width int) []string {
	keyCol := 12
	if width < 30 {
		keyCol = 9
	}
	valWidth := max(8, width-keyCol-3)
	wrapped := wrap(value, valWidth)
	lines := make([]string, 0, len(wrapped))
	for i, v := range wrapped {
		label := ""
		if i == 0 {
			label = key
		}
		lines = append(lines, fmt.Sprintf("%-*s : %s", keyCol, label, v))
	}
	return lines
}

func wrap(s string, width int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{"-"}
	}
	if width <= 6 {
		return []string{trim(s, width)}
	}

	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{trim(s, width)}
	}

	lines := []string{}
	line := words[0]
	for _, w := range words[1:] {
		if lipgloss.Width(line)+1+lipgloss.Width(w) <= width {
			line += " " + w
			continue
		}
		lines = append(lines, line)
		line = w
	}
	lines = append(lines, line)
	for i := range lines {
		lines[i] = trim(lines[i], width)
	}
	return lines
}

func fitLines(lines []string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	out := append([]string(nil), lines[:maxLines]...)
	out[maxLines-1] = trim(out[maxLines-1], max(4, lipgloss.Width(out[maxLines-1])-1)) + "…"
	return out
}

func joinLeftRight(left, right string, width int) string {
	left = trim(left, width)
	right = trim(right, width)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap <= 1 {
		return trim(left+" "+right, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func shortcutTwoColLeftLine(left, right string, gridW, gap int) string {
	if gridW <= 0 {
		return ""
	}
	if right == "" {
		return trim(left, gridW)
	}
	if gap < 1 {
		gap = 1
	}
	usable := gridW - gap
	if usable < 16 {
		return trim(left+" "+right, gridW)
	}
	leftW := usable / 2
	rightW := usable - leftW

	leftCol := lipgloss.NewStyle().Width(leftW).Align(lipgloss.Left).Render(trim(left, leftW))
	rightCol := lipgloss.NewStyle().Width(rightW).Align(lipgloss.Left).Render(trim(right, rightW))
	return leftCol + strings.Repeat(" ", gap) + rightCol
}

func formatShortcutEntry(key, action string, keyW int) string {
	key = strings.TrimSpace(key)
	action = strings.TrimSpace(action)
	if keyW < lipgloss.Width(key) {
		keyW = lipgloss.Width(key)
	}
	return key + strings.Repeat(" ", max(0, keyW-lipgloss.Width(key))) + " : " + action
}

func shortcutMaxKeyWidth(entries []shortcutEntry) int {
	maxW := 0
	for _, entry := range entries {
		if w := lipgloss.Width(strings.TrimSpace(entry.key)); w > maxW {
			maxW = w
		}
	}
	return maxW
}

func (a *App) triggerSearch() tea.Cmd {
	a.lastRequestID++
	id := a.lastRequestID
	return a.debounceCmd(id)
}

func (a *App) debounceCmd(id int) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return debounceMsg{id: id}
	})
}

func (a *App) runSearch(id int) tea.Cmd {
	q := a.query
	f := a.filters
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.status = statusSearching

	return func() tea.Msg {
		pkgs, err := a.service.Search(ctx, q, f)
		return searchMsg{id: id, pkgs: pkgs, err: err}
	}
}

func (a *App) selected() (models.Package, bool) {
	if len(a.pkgs) == 0 || a.cursor < 0 || a.cursor >= len(a.pkgs) {
		return models.Package{}, false
	}
	return a.pkgs[a.cursor], true
}

func (a *App) handleCopy(value string) {
	a.copied = value
	if err := clipboard.Write(value); err != nil {
		a.copyErr = err.Error()
		return
	}
	a.copyErr = ""
}

func (a *App) handleActionKey(key string) bool {
	switch key {
	case "alt+c":
		if p, ok := a.selected(); ok {
			a.handleCopy(actions.AttrPath(p.AttrPath))
		}
		return true
	case "alt+i":
		if p, ok := a.selected(); ok {
			a.handleCopy(actions.NixProfileInstall(p.AttrPath))
		}
		return true
	case "alt+r":
		if p, ok := a.selected(); ok {
			a.handleCopy(actions.NixRun(p.AttrPath))
		}
		return true
	default:
		return false
	}
}

func (a *App) resizeInput() {
	a.input.Width = max(12, a.contentWidth()-14)
}

func (a *App) contentWidth() int {
	return max(40, a.width-2)
}

func (a *App) contentHeight() int {
	return max(10, a.height)
}

func (a *App) wrapScreen(content string, contentW, contentH int) string {
	inner := containerStyle.Width(contentW).Height(contentH).Render(content)
	return screenStyle.Width(max(1, a.width)).Height(max(1, a.height)).Render(inner)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return "-"
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func installedLabel(installed bool) string {
	if installed {
		return "yes"
	}
	return "no"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func trim(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
