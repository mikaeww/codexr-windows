package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProcessPreservesArgumentsEnvironmentAndExitCode(t *testing.T) {
	if os.Getenv("CODEXR_PROCESS_TEST") == "1" {
		if os.Args[len(os.Args)-1] != "spaces & literal arguments" {
			os.Exit(12)
		}
		os.Exit(23)
	}
	err := runProcess(os.Args[0], []string{"-test.run=TestProcessPreservesArgumentsEnvironmentAndExitCode", "--", "spaces & literal arguments"}, append(os.Environ(), "CODEXR_PROCESS_TEST=1"))
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 23 {
		t.Fatalf("child exit = %v, want 23", err)
	}
}

func TestWindowsNpmResolution(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows npm layout")
	}
	root := t.TempDir()
	architecture, triple := "x64", "x86_64-pc-windows-msvc"
	if runtime.GOARCH == "arm64" {
		architecture, triple = "arm64", "aarch64-pc-windows-msvc"
	}
	native := filepath.Join(root, "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-"+architecture, "vendor", triple, "bin", "codex.exe")
	if err := os.MkdirAll(filepath.Dir(native), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := preferNativeCodex(filepath.Join(root, "codex.cmd")); got != native {
		t.Fatalf("resolved %q, want %q", got, native)
	}
	explicit := filepath.Join(root, "explicit.exe")
	if got := preferNativeCodex(explicit); got != explicit {
		t.Fatalf("overrode explicit executable: %q", got)
	}
}
