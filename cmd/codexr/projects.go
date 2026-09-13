package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikaeww/codexr-windows/internal/tui"
)

func loadElectronProjects(codexHome string) ([]tui.Project, error) {
	if strings.TrimSpace(codexHome) == "" {
		return nil, errors.New("codex home is required")
	}
	path := filepath.Join(codexHome, ".codex-global-state.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Electron project state: %w", err)
	}

	var globalState struct {
		SavedWorkspaceRoots []string `json:"electron-saved-workspace-roots"`
	}
	if err := json.Unmarshal(data, &globalState); err != nil {
		return nil, fmt.Errorf("decode Electron project state: %w", err)
	}

	projects := make([]tui.Project, 0, len(globalState.SavedWorkspaceRoots))
	seen := make(map[string]struct{}, len(globalState.SavedWorkspaceRoots))
	for _, root := range globalState.SavedWorkspaceRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = resolved
		}
		if _, exists := seen[absolute]; exists {
			continue
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[absolute] = struct{}{}
		name := filepath.Base(absolute)
		if name == "." || name == string(filepath.Separator) {
			name = absolute
		}
		projects = append(projects, tui.Project{Name: name, Path: absolute})
	}
	return projects, nil
}
