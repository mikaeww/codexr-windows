//go:build !windows

package desktop

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestParentOfReadsPastACommWithSpaces(t *testing.T) {
	// A process name containing spaces and parentheses is the classic way to
	// break /proc/<pid>/stat parsers that just split on whitespace.
	command := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	}()

	parent, ok := parentOf(command.Process.Pid)
	if !ok {
		t.Fatal("parentOf failed on a live process")
	}
	if parent != os.Getpid() {
		t.Errorf("parentOf returned %d, want this test process %d", parent, os.Getpid())
	}
}

func TestParentOfRejectsUnknownProcesses(t *testing.T) {
	if _, ok := parentOf(0); ok {
		t.Error("parentOf accepted pid 0")
	}
	if _, ok := parentOf(1 << 30); ok {
		t.Error("parentOf accepted a pid that cannot exist")
	}
}

func TestIsElectronMainIgnoresHelperProcesses(t *testing.T) {
	if isElectronMain(os.Getpid()) {
		t.Error("the test binary was mistaken for an Electron main process")
	}
	if isElectronMain(1 << 30) {
		t.Error("a nonexistent pid was mistaken for an Electron main process")
	}
}

func TestAliveTracksAProcessLifetime(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "exit 0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	_ = command.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && alive(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	if alive(pid) {
		t.Errorf("alive still reports pid %d after it exited and was reaped", pid)
	}
}

func TestStopRefusesAnInstanceWithNoProcess(t *testing.T) {
	if err := Stop(Instance{CodexHome: "/tmp/nowhere"}); err == nil {
		t.Error("Stop accepted an instance with no Electron process")
	}
}

func TestElectronOwnerGivesUpOnUnrelatedTrees(t *testing.T) {
	if owner := electronOwner(os.Getpid()); owner != 0 {
		t.Errorf("electronOwner walked out of its own tree and returned %d", owner)
	}
}
