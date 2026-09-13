package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikaeww/codexr-windows/internal/state"
)

func press(subject model, keys ...string) model {
	for _, key := range keys {
		var message tea.KeyMsg
		switch key {
		case "enter":
			message = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			message = tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			message = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			message = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		next, _ := subject.Update(message)
		subject = next.(model)
	}
	return subject
}

func TestNavigationMovesAndStopsAtTheEnds(t *testing.T) {
	subject := sampleModel()
	subject.cursor = 0

	if got := press(subject, "j", "j").cursor; got != 2 {
		t.Errorf("j j moved the cursor to %d, want 2", got)
	}
	if got := press(subject, "k").cursor; got != 0 {
		t.Errorf("k moved past the top to %d, want 0", got)
	}
	if got := press(subject, "j", "j", "j", "j", "j", "j").cursor; got != len(subject.rows)-1 {
		t.Errorf("cursor ran past the last row to %d", got)
	}
}

func TestStartRequiresASignedInAccount(t *testing.T) {
	subject := sampleModel()
	subject.cursor = 2 // "Spare", not signed in

	blocked := press(subject, "s")
	if blocked.result.Launch != nil {
		t.Fatal("s launched Codex on an account that is not signed in")
	}
	if !strings.Contains(blocked.status, "sign in first") {
		t.Errorf("no useful message after blocking the launch: %q", blocked.status)
	}
}

func TestStartOpensProjectPickerForSignedInAccount(t *testing.T) {
	subject := sampleModel()
	subject.options.Projects = []Project{
		{Name: "First Project", Path: "/tmp/first"},
		{Name: "Second Project", Path: "/tmp/second"},
	}
	subject.cursor = 1

	selected := press(subject, "s")
	if selected.mode != modeProjectList {
		t.Fatalf("s opened mode %v, want project picker", selected.mode)
	}
	if selected.launch.Label != "Work" {
		t.Fatalf("project picker selected account %q, want Work", selected.launch.Label)
	}
	if selected.projectCursor != 0 {
		t.Fatalf("project picker started at %d, want 0", selected.projectCursor)
	}
}

func TestProjectSelectionReturnsAccountAndDirectory(t *testing.T) {
	subject := sampleModel()
	subject.mode = modeProjectList
	subject.launch = state.Account{ID: "b", Label: "Work"}
	subject.options.Projects = []Project{{Name: "Second Project", Path: "/tmp/second"}}

	updated, _ := subject.Update(projectSelectedMsg{
		account: subject.launch,
		project: subject.options.Projects[0].Path,
	})
	result := updated.(model).result
	if result.Launch == nil || result.Launch.ID != "b" {
		t.Fatalf("launch account = %#v, want Work", result.Launch)
	}
	if result.Project != "/tmp/second" {
		t.Fatalf("launch project = %q, want /tmp/second", result.Project)
	}
}

func TestAddPromptCollectsALabelAndEscapeCancels(t *testing.T) {
	subject := press(sampleModel(), "a")
	if subject.mode != modeAddLabel {
		t.Fatal("a did not open the add prompt")
	}
	typed := press(subject, "P", "r", "o")
	if typed.input != "Pro" {
		t.Errorf("prompt collected %q, want \"Pro\"", typed.input)
	}
	if got := press(typed, "backspace").input; got != "Pr" {
		t.Errorf("backspace left %q, want \"Pr\"", got)
	}
	cancelled := press(typed, "esc")
	if cancelled.mode != modeList || cancelled.input != "" {
		t.Errorf("esc did not cancel the prompt: mode=%v input=%q", cancelled.mode, cancelled.input)
	}
}

func TestRenamePrefillsTheCurrentLabel(t *testing.T) {
	subject := sampleModel()
	subject.cursor = 1

	renaming := press(subject, "n")
	if renaming.mode != modeRenameLabel {
		t.Fatal("n did not open the rename prompt")
	}
	if renaming.input != "Work" {
		t.Errorf("rename prompt started with %q, want the current label", renaming.input)
	}
	if renaming.pending != "b" {
		t.Errorf("rename targets account %q, want the selected one", renaming.pending)
	}
	if !strings.Contains(renaming.View(), "Work") {
		t.Error("rename prompt does not show the label being edited")
	}
}

func TestLogoutAsksBeforeSigningOut(t *testing.T) {
	subject := sampleModel()
	subject.cursor = 0

	asking := press(subject, "o")
	if asking.mode != modeConfirmLogout {
		t.Fatal("o did not ask for confirmation")
	}
	if got := press(asking, "n").mode; got != modeList {
		t.Errorf("any key other than y should cancel, mode is %v", got)
	}

	notSignedIn := sampleModel()
	notSignedIn.cursor = 2
	if got := press(notSignedIn, "o").mode; got == modeConfirmLogout {
		t.Error("o asked to sign out an account that is not signed in")
	}
}

func TestEscapeLeavesDeviceCodeRunningInTheBackground(t *testing.T) {
	subject := sampleModel()
	subject.mode = modeDeviceCode
	subject.device = deviceCode{label: "Work", url: "https://example.com", code: "ABCD"}

	backgrounded := press(subject, "esc")
	if backgrounded.mode != modeList {
		t.Fatal("esc did not leave the device-code screen")
	}
	if !strings.Contains(backgrounded.status, "background") {
		t.Errorf("user was not told the sign-in kept running: %q", backgrounded.status)
	}
}

func TestQuitSetsNoLaunch(t *testing.T) {
	subject := press(sampleModel(), "q")
	if !subject.quit {
		t.Fatal("q did not quit")
	}
	if subject.result.Launch != nil {
		t.Error("quitting requested a launch")
	}
}
