package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mikaeww/codexr-windows/internal/desktop"
	"github.com/mikaeww/codexr-windows/internal/mux"
	"github.com/mikaeww/codexr-windows/internal/state"
)

type Options struct {
	Multiplexer *mux.Multiplexer
	Store       *state.Store
	Project     string
	Projects    []Project
	Version     string
}

type Project struct {
	Name string
	Path string
}

// Result carries an action the terminal UI cannot perform itself. Launching
// Codex replaces the process image, which has to happen after the alternate
// screen is torn down.
type Result struct {
	Launch  *state.Account
	Project string
}

type mode int

const (
	modeList mode = iota
	modeAddLabel
	modeRenameLabel
	modeDeviceCode
	modeConfirmLogout
	modeProjectList
)

type row struct {
	snapshot mux.AccountSnapshot
	account  state.Account
	projects int
	desktop  bool
}

type model struct {
	options Options
	rows    []row
	cursor  int
	owner   string

	mode    mode
	input   string
	pending string

	device        deviceCode
	deviceStart   time.Time
	launch        state.Account
	projectCursor int

	status     string
	statusKind statusKind
	busy       string
	spinner    int

	width  int
	height int

	result Result
	quit   bool
}

type statusKind int

const (
	statusInfo statusKind = iota
	statusGood
	statusBad
)

type accountsMsg struct {
	rows  []row
	owner string
	err   error
}

type actionMsg struct {
	message string
	kind    statusKind
	reload  bool
}

type deviceCode struct {
	accountID string
	label     string
	url       string
	code      string
}

type deviceStartedMsg struct {
	device deviceCode
	err    error
}

type deviceCompletedMsg struct {
	label string
	email string
	plan  string
	err   error
}

type projectSelectedMsg struct {
	account state.Account
	project string
	err     error
}

type tickMsg time.Time

func newModel(options Options) model {
	return model{
		options: options,
		busy:    "starting subscriptions",
		status:  "",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadAccounts(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) loadAccounts() tea.Cmd {
	multiplexer := m.options.Multiplexer
	store := m.options.Store
	project := m.options.Project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		snapshots := multiplexer.Accounts(ctx)
		counts := store.ProjectCounts()
		open := make(map[string]bool, 4)
		for _, instance := range desktop.Running() {
			open[instance.CodexHome] = true
		}
		rows := make([]row, 0, len(snapshots))
		for _, snapshot := range snapshots {
			account, _ := store.Account(snapshot.ID)
			rows = append(rows, row{
				snapshot: snapshot,
				account:  account,
				projects: counts[snapshot.ID],
				desktop:  open[filepath.Clean(account.CodexHome)],
			})
		}
		owner, _ := store.ProjectOwner(project)
		return accountsMsg{rows: rows, owner: owner}
	}
}

func (m model) switchTo(account state.Account) tea.Cmd {
	store := m.options.Store
	project := m.options.Project
	return func() tea.Msg {
		if err := store.SetProjectOwner(project, account.ID); err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		return actionMsg{
			message: fmt.Sprintf("this directory now uses %s", account.Label),
			kind:    statusGood,
			reload:  true,
		}
	}
}

func (m model) autoPick() tea.Cmd {
	multiplexer := m.options.Multiplexer
	store := m.options.Store
	project := m.options.Project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		account, reason, err := multiplexer.PickAccount(ctx, nil)
		if err != nil {
			if errors.Is(err, mux.ErrNoCapacity()) {
				return actionMsg{
					message: "every enabled subscription is depleted",
					kind:    statusBad,
				}
			}
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		if err := store.SetProjectOwner(project, account.ID); err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		return actionMsg{
			message: fmt.Sprintf("routed to %s — %s", account.Label, describeReason(reason)),
			kind:    statusGood,
			reload:  true,
		}
	}
}

func (m model) addAccount(label string) tea.Cmd {
	multiplexer := m.options.Multiplexer
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		account, err := multiplexer.AddAccount(ctx, label)
		if err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		return actionMsg{
			message: fmt.Sprintf("added %s — press l to sign in", account.Label),
			kind:    statusGood,
			reload:  true,
		}
	}
}

func (m model) renameAccount(accountID, label string) tea.Cmd {
	multiplexer := m.options.Multiplexer
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		account, err := multiplexer.UpdateAccount(ctx, accountID, &label, nil)
		if err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		return actionMsg{
			message: fmt.Sprintf("renamed to %s", account.Label),
			kind:    statusGood,
			reload:  true,
		}
	}
}

func (m model) setEnabled(accountID string, enabled bool) tea.Cmd {
	multiplexer := m.options.Multiplexer
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		account, err := multiplexer.UpdateAccount(ctx, accountID, nil, &enabled)
		if err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		verb := "excluded from routing"
		if enabled {
			verb = "included in routing"
		}
		return actionMsg{
			message: fmt.Sprintf("%s is now %s", account.Label, verb),
			kind:    statusGood,
			reload:  true,
		}
	}
}

func (m model) logout(accountID string) tea.Cmd {
	multiplexer := m.options.Multiplexer
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := multiplexer.Logout(ctx, accountID); err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		return actionMsg{message: "signed out", kind: statusGood, reload: true}
	}
}

func (m model) startLogin(account state.Account) tea.Cmd {
	multiplexer := m.options.Multiplexer
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		raw, err := multiplexer.StartLogin(ctx, account.ID, "chatgptDeviceCode")
		if err != nil {
			return deviceStartedMsg{err: err}
		}
		login, err := decodeDeviceCode(raw)
		if err != nil {
			return deviceStartedMsg{err: err}
		}
		login.accountID = account.ID
		login.label = account.Label
		return deviceStartedMsg{device: login}
	}
}

func (m model) awaitLogin(device deviceCode) tea.Cmd {
	multiplexer := m.options.Multiplexer
	return func() tea.Msg {
		deadline := time.Now().Add(10 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			snapshot, err := multiplexer.Account(ctx, device.accountID)
			cancel()
			if err != nil {
				continue
			}
			if snapshot.Connected {
				return deviceCompletedMsg{
					label: device.label,
					email: snapshot.Email,
					plan:  planLabel(snapshot),
				}
			}
		}
		return deviceCompletedMsg{err: errors.New("timed out waiting for the sign-in")}
	}
}

func (m *model) selected() (state.Account, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return state.Account{}, false
	}
	account := m.rows[m.cursor].account
	return account, account.ID != ""
}

// switchDesktop moves the desktop app onto another account. The app reads its
// account once, at launch, so a running window has to be restarted for the
// switch to show up in it.
func (m model) switchDesktop(account state.Account) tea.Cmd {
	return func() tea.Msg {
		if _, ok := desktop.RunningFor(account.CodexHome); ok {
			return actionMsg{
				message: fmt.Sprintf("Codex Desktop is already running as %s", account.Label),
				kind:    statusInfo,
			}
		}
		if len(desktop.Running()) == 0 {
			if err := desktop.Launch(account.CodexHome, false); err != nil {
				return actionMsg{message: err.Error(), kind: statusBad}
			}
			return actionMsg{
				message: "opening Codex Desktop as " + account.Label,
				kind:    statusGood,
				reload:  true,
			}
		}
		if err := desktop.Restart(account.CodexHome); err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		return actionMsg{
			message: "restarted Codex Desktop as " + account.Label,
			kind:    statusGood,
			reload:  true,
		}
	}
}

func (m model) openDesktopWindow(account state.Account) tea.Cmd {
	return func() tea.Msg {
		if err := desktop.Launch(account.CodexHome, len(desktop.Running()) > 0); err != nil {
			return actionMsg{message: err.Error(), kind: statusBad}
		}
		return actionMsg{
			message: "opening another Codex Desktop window as " + account.Label,
			kind:    statusGood,
			reload:  true,
		}
	}
}
