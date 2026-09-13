package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreBootstrapsPrimaryAndPersistsThreadAffinity(t *testing.T) {
	root := t.TempDir()
	primaryHome := filepath.Join(root, "primary")
	store, err := Open(filepath.Join(root, "mux"), primaryHome)
	if err != nil {
		t.Fatal(err)
	}
	accounts := store.Accounts()
	if len(accounts) != 1 || accounts[0].ID != "primary" || !accounts[0].Controller {
		t.Fatalf("unexpected bootstrap accounts: %#v", accounts)
	}
	added, err := store.AddAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(added.CodexHome, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	wantConfig := "cli_auth_credentials_store = \"file\"\nmcp_oauth_credentials_store = \"file\"\n"
	if string(config) != wantConfig {
		t.Fatalf("unexpected isolated config: %q", config)
	}
	if err := store.SetThreadOwner("thread-1", added.ID); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(filepath.Join(root, "mux"), primaryHome)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := reopened.ThreadOwner("thread-1")
	if !ok || owner != added.ID {
		t.Fatalf("thread affinity was not persisted: owner=%q ok=%v", owner, ok)
	}
}

func TestProjectAffinityPersistsAndReassigns(t *testing.T) {
	root := t.TempDir()
	primaryHome := filepath.Join(root, "primary")
	muxRoot := filepath.Join(root, "mux")
	store, err := Open(muxRoot, primaryHome)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AddAccount("First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AddAccount("Second")
	if err != nil {
		t.Fatal(err)
	}

	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.ProjectOwner(project); ok {
		t.Fatal("unassigned project reported an owner")
	}
	if err := store.SetProjectOwner(project, first.ID); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(muxRoot, primaryHome)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := reopened.ProjectOwner(project)
	if !ok || owner != first.ID {
		t.Fatalf("project affinity was not persisted: owner=%q ok=%v", owner, ok)
	}

	relative := filepath.Join(project, "nested", "..")
	if owner, ok := reopened.ProjectOwner(relative); !ok || owner != first.ID {
		t.Fatalf("equivalent path did not resolve to the same owner: owner=%q ok=%v", owner, ok)
	}

	if err := reopened.SetProjectOwner(project, second.ID); err != nil {
		t.Fatal(err)
	}
	if counts := reopened.ProjectCounts(); counts[second.ID] != 1 || counts[first.ID] != 0 {
		t.Fatalf("failover did not move the project: %#v", counts)
	}
	if err := reopened.ClearProjectOwner(project); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.ProjectOwner(project); ok {
		t.Fatal("cleared project still reports an owner")
	}
}

func TestStoreReadsUpstreamStateWithoutProjectOwners(t *testing.T) {
	root := t.TempDir()
	muxRoot := filepath.Join(root, "mux")
	if err := os.MkdirAll(muxRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	primaryJSON, err := json.Marshal(filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	upstream := `{"version":1,"accounts":[{"id":"primary","label":"Primary","codexHome":` +
		string(primaryJSON) + `,"enabled":true,"controller":true,"createdAt":1}],` +
		`"threadOwner":{"thread-1":"primary"}}`
	if err := os.WriteFile(filepath.Join(muxRoot, "state.json"), []byte(upstream), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(muxRoot, filepath.Join(root, "primary"))
	if err != nil {
		t.Fatalf("upstream state was rejected: %v", err)
	}
	if owner, ok := store.ThreadOwner("thread-1"); !ok || owner != "primary" {
		t.Fatalf("upstream thread affinity was lost: owner=%q ok=%v", owner, ok)
	}
	if err := store.SetProjectOwner(root, "primary"); err != nil {
		t.Fatal(err)
	}
}

func TestAccountConfigInheritsManagedMCPAndPreservesLocalProjects(t *testing.T) {
	root := t.TempDir()
	primaryHome := filepath.Join(root, "primary")
	if err := os.MkdirAll(primaryHome, 0o700); err != nil {
		t.Fatal(err)
	}
	primaryConfig := `model = "gpt-test"

[mcp_servers.node_repl]
command = "/home/user/.local/bin/node_repl"

[mcp_servers.node_repl.env]
NODE_REPL_ROOT = "/home/user/.local/share/node_repl"

[projects."/primary-only"]
trust_level = "trusted"
`
	if err := os.WriteFile(filepath.Join(primaryHome, "config.toml"), []byte(primaryConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	muxRoot := filepath.Join(root, "mux")
	store, err := Open(muxRoot, primaryHome)
	if err != nil {
		t.Fatal(err)
	}
	added, err := store.AddAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(added.CodexHome, "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	for _, expected := range []string{
		`cli_auth_credentials_store = "file"`,
		`mcp_oauth_credentials_store = "file"`,
		`model = "gpt-test"`,
		`[mcp_servers.node_repl]`,
		`NODE_REPL_ROOT = "/home/user/.local/share/node_repl"`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("account config is missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "/primary-only") {
		t.Fatalf("primary project trust leaked into account config:\n%s", text)
	}

	text += `
[projects."/account-project"]
trust_level = "trusted"
`
	if err := os.WriteFile(configPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	primaryConfig = strings.ReplaceAll(primaryConfig, "gpt-test", "gpt-updated")
	if err := os.WriteFile(filepath.Join(primaryHome, "config.toml"), []byte(primaryConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(muxRoot, primaryHome); err != nil {
		t.Fatal(err)
	}
	config, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text = string(config)
	if !strings.Contains(text, `model = "gpt-updated"`) {
		t.Fatalf("managed config was not refreshed:\n%s", text)
	}
	if !strings.Contains(text, `[projects."/account-project"]`) {
		t.Fatalf("account project trust was not preserved:\n%s", text)
	}
}

func TestSyncManagedConfigPropagatesPluginsWithoutRestart(t *testing.T) {
	root := t.TempDir()
	primaryHome := filepath.Join(root, "primary")
	if err := os.MkdirAll(primaryHome, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(primaryHome, "config.toml")
	if err := os.WriteFile(configPath, []byte("model = \"before\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "mux"), primaryHome)
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.AddAccount("Work")
	if err != nil {
		t.Fatal(err)
	}
	updated := "model = \"after\"\n\n[plugins.\"browser@openai-bundled\"]\nenabled = true\n"
	if err := os.WriteFile(configPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncManagedConfig(); err != nil {
		t.Fatal(err)
	}
	isolated, err := os.ReadFile(filepath.Join(account.CodexHome, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(isolated), `[plugins."browser@openai-bundled"]`) {
		t.Fatalf("plugin config did not propagate:\n%s", isolated)
	}
}

func TestUpdateAccountPreservesController(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root, filepath.Join(root, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	label := "Personal"
	enabled := false
	account, err := store.UpdateAccount("primary", &label, &enabled)
	if err != nil {
		t.Fatal(err)
	}
	if account.Label != label || account.Enabled || !account.Controller {
		t.Fatalf("unexpected updated account: %#v", account)
	}
}
