package backend

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type safeBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (s *safeBuffer) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.Write(data)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.String()
}

// A child app-server logs on its own schedule. If that output goes to the
// terminal it paints straight over any full-screen UI, so it has to land
// wherever the caller says — never on os.Stderr by default.
func TestChildLogsGoToTheCallersWriter(t *testing.T) {
	collected := &safeBuffer{}
	inbound := make(chan Inbound, 4)

	child, err := Start(
		"test",
		t.TempDir(),
		os.Args[0],
		[]string{"-test.run=TestChildProcess", "--", "diagnostics"},
		[]string{},
		inbound,
		collected,
	)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-child.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("child did not exit")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(collected.String(), "failed to refresh") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child stderr did not reach the caller's writer, got %q", collected.String())
}

func TestChildToleratesNoWriter(t *testing.T) {
	inbound := make(chan Inbound, 4)
	child, err := Start(
		"test",
		t.TempDir(),
		os.Args[0],
		[]string{"-test.run=TestChildProcess", "--", "diagnostics"},
		[]string{},
		inbound,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-child.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("child did not exit with a nil diagnostics writer")
	}
}

func TestChildIsolatesTheAccountHome(t *testing.T) {
	home := t.TempDir()
	collected := &safeBuffer{}
	inbound := make(chan Inbound, 4)

	child, err := Start(
		"test",
		home,
		os.Args[0],
		[]string{"-test.run=TestChildProcess", "--", "environment"},
		[]string{"CODEX_HOME=/should/be/replaced", "CODEX_SQLITE_HOME=/should/be/replaced"},
		inbound,
		collected,
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-child.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("child did not exit")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output := collected.String()
		if strings.Count(output, home) == 2 {
			if strings.Contains(output, "/should/be/replaced") {
				t.Fatalf("an inherited account home survived: %q", output)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child did not run on the isolated home, got %q", collected.String())
}

func TestChildProcess(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	if os.Args[len(os.Args)-1] == "environment" {
		fmt.Fprintln(os.Stderr, os.Getenv("CODEX_HOME"))
		fmt.Fprintln(os.Stderr, os.Getenv("CODEX_SQLITE_HOME"))
	} else {
		fmt.Fprintln(os.Stderr, "ERROR failed to refresh available models")
	}
	os.Exit(0)
}
