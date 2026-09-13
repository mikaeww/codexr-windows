package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mikaeww/codexr-windows/internal/mux"
)

// Run opens the account switcher. It returns once the user quits; a launch
// request is handed back to the caller because replacing the process image has
// to happen after the alternate screen is restored.
func Run(ctx context.Context, options Options) (Result, error) {
	program := tea.NewProgram(newModel(options), tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := program.Run()
	if err != nil {
		return Result{}, err
	}
	if finished, ok := final.(model); ok {
		return finished.result, nil
	}
	return Result{}, nil
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = typed.Width, typed.Height
		return m, nil

	case tickMsg:
		m.spinner++
		return m, tick()

	case accountsMsg:
		m.busy = ""
		if typed.err != nil {
			m.status, m.statusKind = typed.err.Error(), statusBad
			return m, nil
		}
		m.rows = typed.rows
		m.owner = typed.owner
		if m.cursor >= len(m.rows) {
			m.cursor = maxInt(0, len(m.rows)-1)
		}
		return m, nil

	case actionMsg:
		m.busy = ""
		m.status, m.statusKind = typed.message, typed.kind
		if typed.reload {
			m.busy = "refreshing"
			return m, m.loadAccounts()
		}
		return m, nil

	case deviceStartedMsg:
		m.busy = ""
		if typed.err != nil {
			m.status, m.statusKind = typed.err.Error(), statusBad
			m.mode = modeList
			return m, nil
		}
		m.device = typed.device
		m.deviceStart = time.Now()
		m.mode = modeDeviceCode
		return m, m.awaitLogin(typed.device)

	case deviceCompletedMsg:
		m.mode = modeList
		if typed.err != nil {
			m.status, m.statusKind = typed.err.Error(), statusBad
			return m, nil
		}
		m.status = fmt.Sprintf("connected %s as %s (%s)", typed.label, typed.email, typed.plan)
		m.statusKind = statusGood
		m.busy = "refreshing"
		return m, m.loadAccounts()

	case projectSelectedMsg:
		if typed.err != nil {
			m.mode = modeList
			m.status, m.statusKind = typed.err.Error(), statusBad
			return m, nil
		}
		m.result.Launch = &typed.account
		m.result.Project = typed.project
		m.quit = true
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(typed)
	}
	return m, nil
}

func (m model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeAddLabel, modeRenameLabel:
		return m.handleTextInput(key)
	case modeConfirmLogout:
		return m.handleLogoutConfirm(key)
	case modeProjectList:
		return m.handleProjectList(key)
	case modeDeviceCode:
		if key.String() == "esc" || key.String() == "q" {
			m.mode = modeList
			m.status, m.statusKind = "sign-in left running in the background", statusInfo
		}
		return m, nil
	}

	switch key.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
		return m, nil

	case "enter", " ":
		account, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.busy = "switching"
		return m, m.switchTo(account)

	case "s":
		account, ok := m.selected()
		if !ok {
			return m, nil
		}
		if !m.rows[m.cursor].snapshot.Connected {
			m.status, m.statusKind = "sign in first — press l", statusBad
			return m, nil
		}
		if len(m.options.Projects) == 0 {
			m.result.Launch = &account
			m.quit = true
			return m, tea.Quit
		}
		m.launch = account
		m.projectCursor = projectIndex(m.options.Projects, m.options.Project)
		m.mode = modeProjectList
		return m, nil

	case "d":
		account, ok := m.selected()
		if !ok {
			return m, nil
		}
		if !m.rows[m.cursor].snapshot.Connected {
			m.status, m.statusKind = "sign in first — press l", statusBad
			return m, nil
		}
		m.busy = "switching Codex Desktop"
		return m, m.switchDesktop(account)

	case "D":
		account, ok := m.selected()
		if !ok {
			return m, nil
		}
		if !m.rows[m.cursor].snapshot.Connected {
			m.status, m.statusKind = "sign in first — press l", statusBad
			return m, nil
		}
		m.busy = "opening another window"
		return m, m.openDesktopWindow(account)

	case "p":
		m.busy = "routing"
		return m, m.autoPick()

	case "l":
		account, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.busy = "requesting a device code"
		return m, m.startLogin(account)

	case "o":
		if _, ok := m.selected(); !ok {
			return m, nil
		}
		if !m.rows[m.cursor].snapshot.Connected {
			m.status, m.statusKind = "this subscription is not signed in", statusInfo
			return m, nil
		}
		m.mode = modeConfirmLogout
		return m, nil

	case "a":
		m.mode = modeAddLabel
		m.input = ""
		return m, nil

	case "n":
		account, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.mode = modeRenameLabel
		m.pending = account.ID
		m.input = account.Label
		return m, nil

	case "e":
		account, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.busy = "updating"
		return m, m.setEnabled(account.ID, !account.Enabled)

	case "r":
		m.busy = "refreshing"
		return m, m.loadAccounts()
	}
	return m, nil
}

func (m model) handleProjectList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "q":
		m.mode = modeList
		return m, nil
	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
		return m, nil
	case "down", "j":
		if m.projectCursor < len(m.options.Projects)-1 {
			m.projectCursor++
		}
		return m, nil
	case "enter", " ":
		if m.projectCursor < 0 || m.projectCursor >= len(m.options.Projects) {
			return m, nil
		}
		account := m.launch
		project := m.options.Projects[m.projectCursor]
		store := m.options.Store
		return m, func() tea.Msg {
			if err := store.SetProjectOwner(project.Path, account.ID); err != nil {
				return projectSelectedMsg{err: err}
			}
			return projectSelectedMsg{account: account, project: project.Path}
		}
	}
	return m, nil
}

func (m model) handleTextInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.mode = modeList
		m.input = ""
		return m, nil
	case tea.KeyEnter:
		label := strings.TrimSpace(m.input)
		if label == "" {
			m.mode = modeList
			return m, nil
		}
		activeMode := m.mode
		accountID := m.pending
		m.mode = modeList
		m.input = ""
		m.pending = ""
		if activeMode == modeAddLabel {
			m.busy = "creating subscription"
			return m, m.addAccount(label)
		}
		m.busy = "renaming"
		return m, m.renameAccount(accountID, label)
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			runes := []rune(m.input)
			m.input = string(runes[:len(runes)-1])
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.input += string(key.Runes)
		if key.Type == tea.KeySpace {
			m.input += " "
		}
		return m, nil
	}
	return m, nil
}

func (m model) handleLogoutConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y":
		account, ok := m.selected()
		m.mode = modeList
		if !ok {
			return m, nil
		}
		m.busy = "signing out"
		return m, m.logout(account.ID)
	default:
		m.mode = modeList
		return m, nil
	}
}

func decodeDeviceCode(raw json.RawMessage) (deviceCode, error) {
	var decoded struct {
		VerificationURL string `json:"verificationUrl"`
		UserCode        string `json:"userCode"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return deviceCode{}, fmt.Errorf("decode login response: %w", err)
	}
	if decoded.VerificationURL == "" || decoded.UserCode == "" {
		return deviceCode{}, fmt.Errorf("app-server returned no device code")
	}
	return deviceCode{url: decoded.VerificationURL, code: decoded.UserCode}, nil
}

func describeReason(reason mux.RouteReason) string {
	parts := make([]string, 0, 3)
	if reason.WeeklyUsedPercent != nil {
		parts = append(parts, fmt.Sprintf("%.0f%% used", *reason.WeeklyUsedPercent))
	}
	if reason.WeeklyResetsAt != nil {
		parts = append(parts, "resets "+time.Unix(*reason.WeeklyResetsAt, 0).In(time.Local).Format("Mon 15:04"))
	}
	if reason.BankedResetCount != nil && *reason.BankedResetCount > 0 {
		parts = append(parts, fmt.Sprintf("%d banked resets", *reason.BankedResetCount))
	}
	if len(parts) == 0 {
		return "no quota data"
	}
	return strings.Join(parts, ", ")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func projectIndex(projects []Project, current string) int {
	for index, project := range projects {
		if project.Path == current {
			return index
		}
	}
	return 0
}
