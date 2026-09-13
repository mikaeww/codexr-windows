//go:build !windows

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	stopGracePeriod = 20 * time.Second
	stopPollEvery   = 250 * time.Millisecond
)

// electronOwner walks up from an app-server to the Electron main process that
// spawned it. The npm launcher may sit in between, so this climbs until it
// finds an Electron process rather than assuming a fixed depth.
func electronOwner(pid int) int {
	for depth := 0; depth < 4 && pid > 1; depth++ {
		parent, ok := parentOf(pid)
		if !ok {
			return 0
		}
		if isElectronMain(parent) {
			return parent
		}
		pid = parent
	}
	return 0
}

func parentOf(pid int) (int, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	// The comm field is parenthesised and may contain spaces, so fields are
	// counted from after the closing parenthesis: state, then ppid.
	closing := strings.LastIndexByte(string(data), ')')
	if closing < 0 {
		return 0, false
	}
	fields := strings.Fields(string(data)[closing+1:])
	if len(fields) < 2 {
		return 0, false
	}
	parent, err := strconv.Atoi(fields[1])
	if err != nil || parent <= 1 {
		return 0, false
	}
	return parent, true
}

func isElectronMain(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	arguments := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	if len(arguments) == 0 || !strings.Contains(arguments[0], "electron") {
		return false
	}
	// Renderer, GPU, and utility processes all share the binary; only the main
	// process has no --type.
	for _, argument := range arguments[1:] {
		if strings.HasPrefix(argument, "--type=") {
			return false
		}
	}
	return true
}

// Stop asks an instance to close and waits for it to go away. It escalates to
// SIGKILL only after the grace period, because a hard kill costs the user
// whatever the window had not written to disk yet.
func Stop(instance Instance) error {
	if instance.AppPID <= 1 {
		return fmt.Errorf("no Codex Desktop process found for %s", instance.CodexHome)
	}
	process, err := os.FindProcess(instance.AppPID)
	if err != nil {
		return fmt.Errorf("find Codex Desktop process: %w", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		if err == os.ErrProcessDone {
			return nil
		}
		return fmt.Errorf("ask Codex Desktop to close: %w", err)
	}

	deadline := time.Now().Add(stopGracePeriod)
	for time.Now().Before(deadline) {
		if !alive(instance.AppPID) {
			return nil
		}
		time.Sleep(stopPollEvery)
	}
	if err := process.Signal(syscall.SIGKILL); err != nil && err != os.ErrProcessDone {
		return fmt.Errorf("force Codex Desktop to close: %w", err)
	}
	for range 20 {
		if !alive(instance.AppPID) {
			return nil
		}
		time.Sleep(stopPollEvery)
	}
	return fmt.Errorf("Codex Desktop (pid %d) did not exit", instance.AppPID)
}

func alive(pid int) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil
}

// Restart switches the running Codex Desktop over to another account: every
// open instance is closed, then one is started on the new account home. The
// app reads its account once at launch, so restarting is what makes a switch
// visible in the window.
func Restart(codexHome string) error {
	if _, err := Launcher(); err != nil {
		return err
	}
	running := Running()
	for _, instance := range running {
		if err := Stop(instance); err != nil {
			return err
		}
	}
	// The launcher refuses to warm-start while its predecessor still holds the
	// port, and the webview server needs a moment to release it.
	if len(running) > 0 {
		waitForPortsToClear(running)
	}
	return Launch(codexHome, false)
}

func waitForPortsToClear(stopped []Instance) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(Running()) == 0 && noneAlive(stopped) {
			time.Sleep(500 * time.Millisecond)
			return
		}
		time.Sleep(stopPollEvery)
	}
}

func noneAlive(instances []Instance) bool {
	for _, instance := range instances {
		if instance.AppPID > 1 && alive(instance.AppPID) {
			return false
		}
	}
	return true
}
