package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/mikaeww/codexr-windows/internal/mux"
)

var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#0B6BCB", Dark: "#7CC4FF"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B93A1"}
	colorGood   = lipgloss.AdaptiveColor{Light: "#137333", Dark: "#6EE7A8"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#F5C161"}
	colorBad    = lipgloss.AdaptiveColor{Light: "#B3261E", Dark: "#FF9B93"}

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleSubtitle = lipgloss.NewStyle().Foreground(colorMuted)
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colorMuted)
	styleSelected = lipgloss.NewStyle().Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleGood     = lipgloss.NewStyle().Foreground(colorGood)
	styleWarn     = lipgloss.NewStyle().Foreground(colorWarn)
	styleBad      = lipgloss.NewStyle().Foreground(colorBad)
	styleBox      = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 3)
	styleKey = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
)

const spinnerFrames = `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`

func (m model) View() string {
	if m.quit {
		return ""
	}
	switch m.mode {
	case modeDeviceCode:
		return m.viewDeviceCode()
	case modeProjectList:
		return m.viewProjectList()
	case modeAddLabel:
		return m.viewPrompt("Name the new subscription", "It only labels the slot — you sign in next.")
	case modeRenameLabel:
		return m.viewPrompt("Rename this subscription", "")
	case modeConfirmLogout:
		return m.viewLogoutConfirm()
	}
	return m.viewList()
}

func (m model) viewProjectList() string {
	var builder strings.Builder
	builder.WriteString("  " + styleTitle.Render("Choose a project"))
	builder.WriteString(styleSubtitle.Render("  for " + m.launch.Label))
	builder.WriteString("\n\n")
	for index, project := range m.options.Projects {
		marker := " "
		if index == m.projectCursor {
			marker = "▸"
		}
		line := fmt.Sprintf("  %s %-32s %s", marker, project.Name, styleMuted.Render(project.Path))
		if index == m.projectCursor {
			builder.WriteString(styleSelected.Render(line))
		} else {
			builder.WriteString(line)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n  " + keyHelp([][2]string{
		{"↑↓", "move"},
		{"enter", "start Codex"},
		{"esc", "back"},
	}) + "\n")
	return builder.String()
}

func (m model) viewList() string {
	var builder strings.Builder
	builder.WriteString(m.header())
	builder.WriteString("\n\n")

	if len(m.rows) == 0 {
		builder.WriteString(styleMuted.Render("  No subscriptions yet — press a to add one.\n"))
	} else {
		builder.WriteString(m.table())
	}

	builder.WriteString("\n")
	builder.WriteString(m.footer())
	return builder.String()
}

func (m model) header() string {
	title := styleTitle.Render("Easy-G Account Switch")
	subtitle := styleSubtitle.Render(fmt.Sprintf("  %s", shortenPath(m.options.Project)))
	return "  " + title + subtitle
}

func (m model) table() string {
	widths := []int{2, 18, 9, 12, 12, 18, 6, 5, 11}
	head := []string{"", "SUBSCRIPTION", "PLAN", "5H", "WEEKLY", "RESETS", "DIRS", "APP", "STATE"}
	var builder strings.Builder
	builder.WriteString("  ")
	for index, cell := range head {
		builder.WriteString(styleHeader.Render(pad(cell, widths[index])))
	}
	builder.WriteString("\n")

	for index, entry := range m.rows {
		snapshot := entry.snapshot
		marker := " "
		if snapshot.ID == m.owner {
			marker = "▸"
		}
		cells := []string{
			marker,
			truncate(snapshot.Label, widths[1]-1),
			truncate(planLabel(snapshot), widths[2]-1),
			short(snapshot),
			weekly(snapshot),
			resets(snapshot),
			fmt.Sprintf("%d", entry.projects),
			desktopCell(entry.desktop),
			"",
		}
		line := "  "
		for column, cell := range cells[:len(cells)-1] {
			line += pad(cell, widths[column])
		}
		line += stateCell(snapshot)

		if index == m.cursor {
			builder.WriteString(styleSelected.Render("▍") + line[1:])
		} else {
			builder.WriteString(line)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func (m model) footer() string {
	var builder strings.Builder

	if m.busy != "" {
		frames := []rune(spinnerFrames)
		frame := string(frames[m.spinner%len(frames)])
		builder.WriteString("  " + styleWarn.Render(frame+" "+m.busy) + "\n")
	} else if m.status != "" {
		style := styleMuted
		switch m.statusKind {
		case statusGood:
			style = styleGood
		case statusBad:
			style = styleBad
		}
		builder.WriteString("  " + style.Render(m.status) + "\n")
	} else {
		builder.WriteString("\n")
	}

	builder.WriteString("\n")
	builder.WriteString("  " + keyHelp([][2]string{
		{"↑↓", "move"},
		{"enter", "switch"},
		{"s", "start codex"},
		{"d", "switch app"},
		{"p", "auto-route"},
		{"l", "login"},
	}))
	builder.WriteString("\n  " + keyHelp([][2]string{
		{"a", "add"},
		{"n", "rename"},
		{"e", "enable/disable"},
		{"o", "logout"},
		{"D", "new app window"},
		{"r", "refresh"},
		{"q", "quit"},
	}))
	builder.WriteString("\n")
	return builder.String()
}

func keyHelp(pairs [][2]string) string {
	rendered := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		rendered = append(rendered, styleKey.Render(pair[0])+" "+styleMuted.Render(pair[1]))
	}
	return strings.Join(rendered, styleMuted.Render("  ·  "))
}

func (m model) viewDeviceCode() string {
	elapsed := time.Since(m.deviceStart).Truncate(time.Second)
	frames := []rune(spinnerFrames)
	frame := string(frames[m.spinner%len(frames)])

	body := strings.Join([]string{
		styleTitle.Render("Sign in to " + m.device.label),
		"",
		styleMuted.Render("1.  Open this page in your browser"),
		"    " + styleKey.Render(m.device.url),
		"",
		styleMuted.Render("2.  Enter this code"),
		"    " + styleTitle.Render(m.device.code),
		"",
		styleMuted.Render("Use the ChatGPT account whose subscription should back this slot."),
		styleMuted.Render("Already signed in as someone else? Use a private window."),
		"",
		styleWarn.Render(fmt.Sprintf("%s waiting… %s", frame, elapsed)),
		styleMuted.Render("esc puts this in the background"),
	}, "\n")

	return "\n" + lipgloss.NewStyle().MarginLeft(2).Render(styleBox.Render(body)) + "\n"
}

func (m model) viewPrompt(title, hint string) string {
	lines := []string{styleTitle.Render(title), ""}
	if hint != "" {
		lines = append(lines, styleMuted.Render(hint), "")
	}
	lines = append(lines,
		"  "+m.input+styleKey.Render("▏"),
		"",
		styleMuted.Render("enter confirms  ·  esc cancels"),
	)
	return "\n" + lipgloss.NewStyle().MarginLeft(2).Render(styleBox.Render(strings.Join(lines, "\n"))) + "\n"
}

func (m model) viewLogoutConfirm() string {
	label := ""
	if m.cursor < len(m.rows) {
		label = m.rows[m.cursor].snapshot.Label
	}
	body := strings.Join([]string{
		styleTitle.Render("Sign out of " + label + "?"),
		"",
		styleMuted.Render("The slot and its settings stay; only the credentials are removed."),
		"",
		styleKey.Render("y") + styleMuted.Render(" sign out  ·  any other key cancels"),
	}, "\n")
	return "\n" + lipgloss.NewStyle().MarginLeft(2).Render(styleBox.Render(body)) + "\n"
}

func planLabel(snapshot mux.AccountSnapshot) string {
	if snapshot.PlanLabel != "" {
		return snapshot.PlanLabel
	}
	if snapshot.PlanType != "" {
		return snapshot.PlanType
	}
	return "—"
}

func weekly(snapshot mux.AccountSnapshot) string {
	window := snapshot.Weekly()
	if window == nil {
		return "—"
	}
	return fmt.Sprintf("%s %3.0f%%", meter(window.UsedPercent), window.UsedPercent)
}

func short(snapshot mux.AccountSnapshot) string {
	window := snapshot.Short()
	if window == nil {
		return "—"
	}
	return fmt.Sprintf("%s %3.0f%%", meter(window.UsedPercent), window.UsedPercent)
}

func meter(usedPercent float64) string {
	const width = 5
	filled := int((usedPercent/100)*float64(width) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	// A full bar has to mean depleted, so 99% must not round up into one.
	if filled == width && usedPercent < 100 {
		filled = width - 1
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	switch {
	case usedPercent >= 100:
		return styleBad.Render(bar)
	case usedPercent >= 85:
		return styleWarn.Render(bar)
	default:
		return styleGood.Render(bar)
	}
}

func resets(snapshot mux.AccountSnapshot) string {
	window := snapshot.Weekly()
	if window == nil || window.ResetsAt == nil {
		return "—"
	}
	reset := time.Unix(*window.ResetsAt, 0).In(time.Local)
	remaining := time.Until(reset)
	if remaining <= 0 {
		return "due"
	}
	return fmt.Sprintf("%s %s", reset.Format("Mon 15:04"), styleMuted.Render("+"+compact(remaining)))
}

func compact(value time.Duration) string {
	hours := int(value.Hours())
	if hours >= 24 {
		return fmt.Sprintf("%dd", hours/24)
	}
	if hours >= 1 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", int(value.Minutes()))
}

func desktopCell(open bool) string {
	if open {
		return styleGood.Render("open")
	}
	return styleMuted.Render("—")
}

func stateCell(snapshot mux.AccountSnapshot) string {
	switch {
	case snapshot.Error != "":
		return styleBad.Render("error")
	case !snapshot.Enabled:
		return styleMuted.Render("disabled")
	case !snapshot.Connected:
		return styleWarn.Render("needs login")
	case snapshot.Weekly() != nil && snapshot.Weekly().UsedPercent >= 100:
		return styleBad.Render("depleted")
	default:
		return styleGood.Render("ready")
	}
}

func pad(value string, width int) string {
	visible := lipgloss.Width(value)
	if visible >= width {
		return value + " "
	}
	return value + strings.Repeat(" ", width-visible)
}

func truncate(value string, width int) string {
	if width <= 1 || lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}

func shortenPath(path string) string {
	if home, err := homeDirectory(); err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
