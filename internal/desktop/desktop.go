//go:build !windows

// Package desktop drives the Codex Desktop app, which resolves its account
// from CODEX_HOME at launch. Starting it with an isolated account home is
// therefore all it takes to run it as a different user — no patching, no
// signing, and nothing written outside $HOME.
package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// appMarker is exported by the Codex Desktop launcher into everything it
// spawns. Our own app-server children never carry it, which is what makes a
// running desktop instance distinguishable from the multiplexer's pool.
const appMarker = "CODEX_LINUX_APP_ID"

type Instance struct {
	// PID is the app-server the window talks to; AppPID is the Electron main
	// process that owns it and the only one worth signalling, because the
	// launcher tears the rest of the tree down with it.
	PID       int
	AppPID    int
	CodexHome string
	Port      string
}

var launcherCandidates = []string{
	"/opt/codex-desktop/start.sh",
	"/usr/lib/codex-desktop/start.sh",
}

// Launcher resolves the Codex Desktop launcher script.
func Launcher() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CODEX_DESKTOP_LAUNCHER")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("CODEX_DESKTOP_LAUNCHER does not exist: %w", err)
		}
		return configured, nil
	}
	if found, err := exec.LookPath("codex-desktop"); err == nil {
		return found, nil
	}
	for _, candidate := range launcherCandidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New("Codex Desktop is not installed; set CODEX_DESKTOP_LAUNCHER to its launcher")
}

// Running lists the Codex Desktop instances currently up, keyed by the account
// home each one was started with.
func Running() []Instance {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	// One window shows up twice: the npm launcher and the native binary it
	// spawns share an environment. Collapse them onto the lowest PID, which is
	// always the parent.
	seen := make(map[string]int, 4)
	instances := make([]Instance, 0, 4)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		command, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || !strings.Contains(string(command), "app-server") {
			continue
		}
		environment, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ"))
		if err != nil {
			continue
		}
		values := parseEnviron(string(environment))
		if values[appMarker] == "" || values["CODEX_HOME"] == "" {
			continue
		}
		instance := Instance{
			PID:       pid,
			AppPID:    electronOwner(pid),
			CodexHome: filepath.Clean(values["CODEX_HOME"]),
			Port:      values["CODEX_LINUX_WEBVIEW_PORT"],
		}
		key := instance.CodexHome + "\x00" + instance.Port
		if index, duplicate := seen[key]; duplicate {
			if pid < instances[index].PID {
				instances[index] = instance
			}
			continue
		}
		seen[key] = len(instances)
		instances = append(instances, instance)
	}
	return instances
}

// RunningFor reports the instance serving one account home, if any.
func RunningFor(codexHome string) (Instance, bool) {
	target := filepath.Clean(codexHome)
	for _, instance := range Running() {
		if instance.CodexHome == target {
			return instance, true
		}
	}
	return Instance{}, false
}

// Launch starts Codex Desktop on an account home and detaches from it, so
// quitting the switcher does not take the app down with it. When another
// instance is already up, the launcher warm-starts into it unless asked for a
// separate one — which is exactly what switching accounts requires.
func Launch(codexHome string, separateInstance bool) error {
	launcher, err := Launcher()
	if err != nil {
		return err
	}
	if _, err := os.Stat(codexHome); err != nil {
		return fmt.Errorf("account home is unavailable: %w", err)
	}

	arguments := []string{}
	environment := environmentFor(codexHome)
	if separateInstance {
		arguments = append(arguments, "--new-instance")
		environment = append(environment, "CODEX_MULTI_LAUNCH=1")
	}

	command := exec.Command(launcher, arguments...)
	command.Env = environment
	command.Dir = os.TempDir()
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Codex Desktop: %w", err)
	}
	return command.Process.Release()
}

func environmentFor(codexHome string) []string {
	overrides := map[string]string{
		"CODEX_HOME":        codexHome,
		"CODEX_SQLITE_HOME": codexHome,
	}
	// The launcher pins its own CLI when this is set. Inheriting a value aimed
	// at the multiplexer would point the desktop app at the wrong binary.
	dropped := map[string]struct{}{
		"CODEX_CLI_PATH":               {},
		"CODEX_MULTI_LAUNCH":           {},
		"CODEX_LINUX_INSTANCE_ID":      {},
		"CODEX_MUX_PRIMARY_CODEX_HOME": {},
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if _, replaced := overrides[name]; replaced {
			continue
		}
		if _, remove := dropped[name]; remove {
			continue
		}
		environment = append(environment, entry)
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func parseEnviron(blob string) map[string]string {
	values := make(map[string]string, 32)
	for _, entry := range strings.Split(blob, "\x00") {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	return values
}
