package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadElectronProjectsReadsSavedWorkspaceRoots(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "First Project")
	second := filepath.Join(home, "Second Project")
	if err := os.MkdirAll(first, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := filepath.EvalSymlinks(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err = filepath.EvalSymlinks(second)
	if err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(map[string][]string{"electron-saved-workspace-roots": {first, first, second, filepath.Join(home, "missing")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex-global-state.json"), state, 0o600); err != nil {
		t.Fatal(err)
	}

	projects, err := loadElectronProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("loaded %d projects, want 2", len(projects))
	}
	if projects[0].Name != "First Project" || projects[0].Path != first {
		t.Fatalf("first project = %#v", projects[0])
	}
	if projects[1].Name != "Second Project" || projects[1].Path != second {
		t.Fatalf("second project = %#v", projects[1])
	}
}
