package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mikaeww/codexr-windows/internal/mux"
	"github.com/mikaeww/codexr-windows/internal/state"
)

func window(usedPercent float64, resetsIn time.Duration) *mux.RateLimitWindow {
	duration := int64(10080)
	resetsAt := time.Now().Add(resetsIn).Unix()
	return &mux.RateLimitWindow{
		UsedPercent:        usedPercent,
		WindowDurationMins: &duration,
		ResetsAt:           &resetsAt,
	}
}

func sampleModel() model {
	home, err := homeDirectory()
	if err != nil {
		home = "/home/user"
	}
	return model{
		options: Options{Project: filepath.Join(home, "project")},
		owner:   "b",
		cursor:  1,
		rows: []row{
			{account: state.Account{ID: "a", Label: "Primary", Enabled: true}, snapshot: mux.AccountSnapshot{
				ID: "a", Label: "Primary", Enabled: true, Connected: true,
				Email: "one@example.com", AuthType: "chatgpt", PlanLabel: "Plus",
				RateLimits: &mux.RateLimits{Primary: window(100, 48*time.Hour)},
			}},
			{account: state.Account{ID: "b", Label: "Work", Enabled: true}, snapshot: mux.AccountSnapshot{
				ID: "b", Label: "Work", Enabled: true, Connected: true,
				Email: "two@example.com", AuthType: "chatgpt", PlanLabel: "Pro 20x",
				RateLimits: &mux.RateLimits{Primary: window(12, 96*time.Hour)},
			}, projects: 3},
			{account: state.Account{ID: "c", Label: "Spare", Enabled: true}, snapshot: mux.AccountSnapshot{
				ID: "c", Label: "Spare", Enabled: true,
			}},
			{account: state.Account{ID: "d", Label: "Paused"}, snapshot: mux.AccountSnapshot{
				ID: "d", Label: "Paused", Enabled: false, Connected: true,
				Email: "four@example.com", AuthType: "chatgpt",
			}},
		},
	}
}

func TestListViewShowsEveryAccountAndItsState(t *testing.T) {
	rendered := sampleModel().View()

	for _, expected := range []string{
		"Easy-G Account Switch",
		"~" + string(filepath.Separator) + "project",
		"5H",
		"Primary", "Plus", "depleted",
		"Work", "Pro 20x", "ready",
		"Spare", "needs login",
		"Paused", "disabled",
		"switch", "start codex", "auto-route", "login",
	} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("list view is missing %q\n%s", expected, rendered)
		}
	}
}

func TestListViewMarksTheActiveSubscription(t *testing.T) {
	rendered := sampleModel().View()
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "Work") {
			if !strings.Contains(line, "▸") {
				t.Errorf("the directory's own subscription is not marked:\n%s", line)
			}
			return
		}
	}
	t.Fatalf("no row rendered for the active subscription:\n%s", rendered)
}

func TestMeterTracksUsage(t *testing.T) {
	for _, testCase := range []struct {
		usedPercent float64
		filled      int
	}{{0, 0}, {50, 3}, {99, 4}, {99.9, 4}, {100, 5}, {140, 5}} {
		bar := meter(testCase.usedPercent)
		if got := strings.Count(bar, "█"); got != testCase.filled {
			t.Errorf("meter(%v) filled %d blocks, want %d", testCase.usedPercent, got, testCase.filled)
		}
	}
}

func TestDeviceCodeViewShowsUrlAndCode(t *testing.T) {
	subject := sampleModel()
	subject.mode = modeDeviceCode
	subject.device = deviceCode{label: "Work", url: "https://example.com/device", code: "ABCD-1234"}
	subject.deviceStart = time.Now()

	rendered := subject.View()
	for _, expected := range []string{"Sign in to Work", "https://example.com/device", "ABCD-1234", "private window"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("device-code view is missing %q\n%s", expected, rendered)
		}
	}
}

func TestPromptAndLogoutViewsRender(t *testing.T) {
	prompt := sampleModel()
	prompt.mode = modeAddLabel
	prompt.input = "Pro 2"
	if rendered := prompt.View(); !strings.Contains(rendered, "Pro 2") ||
		!strings.Contains(rendered, "esc cancels") {
		t.Errorf("add prompt did not render its input:\n%s", rendered)
	}

	confirm := sampleModel()
	confirm.mode = modeConfirmLogout
	if rendered := confirm.View(); !strings.Contains(rendered, "Sign out of Work?") {
		t.Errorf("logout confirmation did not name the account:\n%s", rendered)
	}
}

func TestTruncateKeepsWidth(t *testing.T) {
	if got := truncate("abcdefghij", 5); got != "abcd…" {
		t.Errorf("truncate returned %q", got)
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate shortened a value that fits: %q", got)
	}
}
