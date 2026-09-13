//go:build !windows

package desktop

import (
	"os"
	"strings"
	"testing"
)

func TestEnvironmentPinsTheAccountHomeAndDropsInheritedOverrides(t *testing.T) {
	t.Setenv("CODEX_HOME", "/home/user/.codex")
	t.Setenv("CODEX_SQLITE_HOME", "/home/user/.codex")
	t.Setenv("CODEX_CLI_PATH", "/home/user/.local/bin/codex-mux")
	t.Setenv("CODEX_MULTI_LAUNCH", "1")
	t.Setenv("PATH", "/usr/bin")

	environment := environmentFor("/home/user/.codex-mux/accounts/abc/codex-home")
	values := map[string]string{}
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		if _, duplicate := values[name]; duplicate {
			t.Fatalf("%s appears twice; the launcher would read the wrong one", name)
		}
		values[name] = value
	}

	want := "/home/user/.codex-mux/accounts/abc/codex-home"
	if values["CODEX_HOME"] != want {
		t.Errorf("CODEX_HOME is %q, want %q", values["CODEX_HOME"], want)
	}
	if values["CODEX_SQLITE_HOME"] != want {
		t.Errorf("CODEX_SQLITE_HOME is %q, want %q", values["CODEX_SQLITE_HOME"], want)
	}
	// The desktop app pins its own CLI from this; ours points at the
	// multiplexer, which is not what the app should spawn.
	if _, leaked := values["CODEX_CLI_PATH"]; leaked {
		t.Error("CODEX_CLI_PATH leaked into the desktop launch")
	}
	if _, leaked := values["CODEX_MULTI_LAUNCH"]; leaked {
		t.Error("CODEX_MULTI_LAUNCH leaked into the desktop launch")
	}
	if values["PATH"] != "/usr/bin" {
		t.Errorf("unrelated variables were not preserved: PATH=%q", values["PATH"])
	}
}

func TestParseEnvironReadsNulSeparatedPairs(t *testing.T) {
	values := parseEnviron("A=1\x00B=two\x00MALFORMED\x00C=3\x00")
	if values["A"] != "1" || values["B"] != "two" || values["C"] != "3" {
		t.Errorf("environ parsed incorrectly: %#v", values)
	}
	if _, present := values["MALFORMED"]; present {
		t.Error("an entry without = was treated as a variable")
	}
}

func TestLauncherPrefersTheConfiguredPath(t *testing.T) {
	script := t.TempDir() + "/start.sh"
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_DESKTOP_LAUNCHER", script)

	found, err := Launcher()
	if err != nil {
		t.Fatal(err)
	}
	if found != script {
		t.Errorf("Launcher returned %q, want the configured path", found)
	}

	t.Setenv("CODEX_DESKTOP_LAUNCHER", script+".missing")
	if _, err := Launcher(); err == nil {
		t.Error("a configured launcher that does not exist was accepted")
	}
}

func TestLaunchRefusesAMissingAccountHome(t *testing.T) {
	t.Setenv("CODEX_DESKTOP_LAUNCHER", "/nonexistent/start.sh")
	if err := Launch(t.TempDir()+"/missing", false); err == nil {
		t.Error("Launch accepted an account home that does not exist")
	}
}
