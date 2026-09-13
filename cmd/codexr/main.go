package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mikaeww/codexr-windows/internal/mux"
	"github.com/mikaeww/codexr-windows/internal/state"
)

const clientName = "codexr"

var version = "0.1.0-windows"

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "codexr: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return commandTUI(nil)
	}
	command, rest := args[0], args[1:]
	switch command {
	case "tui", "switch":
		return commandTUI(rest)
	case "accounts", "ls":
		return commandAccounts(rest)
	case "add":
		return commandAdd(rest)
	case "login":
		return commandLogin(rest)
	case "logout":
		return commandLogout(rest)
	case "rename":
		return commandRename(rest)
	case "enable":
		return commandSetEnabled(rest, true)
	case "disable":
		return commandSetEnabled(rest, false)
	case "desktop", "app":
		return commandDesktop(rest)
	case "pick":
		return commandPick(rest)
	case "run":
		return commandRun(rest)
	case "serve":
		return commandServe(rest)
	case "doctor":
		return commandDoctor(rest)
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Easy-G Account Switch — switch between several ChatGPT subscriptions

Usage:
  codexr                       Open the account switcher (no arguments)
  codexr accounts              Show every subscription with its quota
  codexr add <label>           Create an isolated subscription slot
  codexr login <account>       Sign in with the ChatGPT device-code flow
  codexr logout <account>      Sign out of a subscription
  codexr rename <account> <label>
                               Change a subscription's label
  codexr enable <account>      Include a subscription in routing
  codexr disable <account>     Exclude a subscription from routing
  codexr pick                  Show which subscription a new chat would use
  codexr run [args...]         Launch the real Codex CLI on the routed subscription
  codexr desktop <account>     Switch the Codex Desktop app to that subscription
                               (restarts it, because the app reads its account
                               only at launch)
  codexr desktop <account> --new-window
                               Open another window instead of switching
  codexr desktop --list        Show which subscriptions have a desktop window open
  codexr desktop --stop        Close every Codex Desktop window
  codexr serve                 Expose the control API for app-server clients
  codexr doctor                Check the local installation

An <account> is an ID or a label prefix; "primary" is always the account in ~/.codex.

Environment:
  CODEX_MUX_HOME                 state directory (default ~/.codex-mux)
  CODEX_MUX_PRIMARY_CODEX_HOME   primary Codex home (default ~/.codex)
  CODEX_MUX_REAL_CODEX           path to the real codex binary
  CODEX_MUX_CONTROL_PORT         control API port for serve (default 48123)
`)
}

type session struct {
	multiplexer *mux.Multiplexer
	store       *state.Store
	realCodex   string
	logFile     *os.File
}

// openSession starts the account pool. Child app-servers log continuously and
// on their own schedule, so a caller that owns the whole screen must pass a
// file here; sending that output to the terminal paints over the UI.
func openSession(ctx context.Context, diagnostics io.Writer) (*session, error) {
	realCodex, err := resolveRealCodex()
	if err != nil {
		return nil, err
	}
	store, err := openStore()
	if err != nil {
		return nil, err
	}

	active := &session{store: store, realCodex: realCodex}
	if diagnostics == nil {
		logFile, path, logErr := openDiagnosticsLog(store.Root())
		if logErr != nil {
			return nil, logErr
		}
		active.logFile = logFile
		diagnostics = logFile
		_ = path
	}

	multiplexer, err := mux.Headless(ctx, mux.HeadlessOptions{
		RealExecutable: realCodex,
		Store:          store,
		ClientName:     clientName,
		ClientVersion:  version,
		Diagnostics:    diagnostics,
	})
	if err != nil {
		active.closeLog()
		return nil, err
	}
	active.multiplexer = multiplexer
	return active, nil
}

func (s *session) close() {
	if s.multiplexer != nil {
		s.multiplexer.Close()
	}
	s.closeLog()
}

func (s *session) closeLog() {
	if s.logFile != nil {
		_ = s.logFile.Close()
		s.logFile = nil
	}
}

func diagnosticsLogPath(root string) string {
	return filepath.Join(root, "logs", "codexr.log")
}

func openDiagnosticsLog(root string) (*os.File, string, error) {
	path := diagnosticsLogPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, "", fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("open log file: %w", err)
	}
	return logFile, path, nil
}

func openStore() (*state.Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	root := os.Getenv("CODEX_MUX_HOME")
	if root == "" {
		root = filepath.Join(home, ".codex-mux")
	}
	// Deliberately not CODEX_HOME: codexr runs *inside* a session whose
	// CODEX_HOME already points at a routed subscription, and inheriting that
	// would re-register a secondary account as the primary one.
	primary := os.Getenv("CODEX_MUX_PRIMARY_CODEX_HOME")
	if primary == "" {
		primary = filepath.Join(home, ".codex")
	}
	return state.Open(root, primary)
}

func resolveRealCodex() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CODEX_MUX_REAL_CODEX")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("CODEX_MUX_REAL_CODEX does not exist: %w", err)
		}
		return preferNativeCodex(configured), nil
	}
	if recorded, err := recordedRealCodex(); err == nil && recorded != "" {
		return preferNativeCodex(recorded), nil
	}
	found, err := exec.LookPath("codex")
	if err != nil {
		return "", errors.New("no codex binary found; install codex-cli or set CODEX_MUX_REAL_CODEX")
	}
	resolved, err := filepath.EvalSymlinks(found)
	if err != nil {
		resolved = found
	}
	self, err := os.Executable()
	if err == nil {
		if selfResolved, resolveErr := filepath.EvalSymlinks(self); resolveErr == nil {
			self = selfResolved
		}
		if self == resolved {
			return "", errors.New("the codex on PATH is this router; set CODEX_MUX_REAL_CODEX to the real binary")
		}
	}
	return preferNativeCodex(resolved), nil
}

// preferNativeCodex skips the npm package's Node launcher when the vendored
// native binary is available. The launcher only injects self-update metadata
// before spawning that binary, and one Node process per subscription is a cost
// the router pays on every command.
func preferNativeCodex(path string) string {
	if runtime.GOOS == "windows" {
		if strings.EqualFold(filepath.Ext(path), ".exe") {
			return path
		}
		root := filepath.Join(filepath.Dir(path), "node_modules", "@openai", "codex")
		if strings.EqualFold(filepath.Ext(path), ".js") {
			root = filepath.Dir(filepath.Dir(path))
		}
		architecture, triple := "x64", "x86_64-pc-windows-msvc"
		if runtime.GOARCH == "arm64" {
			architecture, triple = "arm64", "aarch64-pc-windows-msvc"
		}
		for _, vendor := range []string{
			filepath.Join(root, "node_modules", "@openai", "codex-win32-"+architecture, "vendor"),
			filepath.Join(filepath.Dir(root), "codex-win32-"+architecture, "vendor"),
			filepath.Join(root, "vendor"),
		} {
			for _, directory := range []string{"bin", "codex"} {
				candidate := filepath.Join(vendor, triple, directory, "codex.exe")
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate
				}
			}
		}
		return path
	}
	if !strings.HasSuffix(path, ".js") {
		return path
	}
	packageRoot := filepath.Dir(filepath.Dir(path))
	matches, err := filepath.Glob(filepath.Join(
		packageRoot, "node_modules", "@openai", "codex-*", "vendor", "*", "bin", "codex"))
	if err != nil {
		return path
	}
	for _, candidate := range matches {
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return path
}

func recordedRealCodex() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := os.Getenv("CODEX_MUX_HOME")
	if root == "" {
		root = filepath.Join(home, ".codex-mux")
	}
	data, err := os.ReadFile(filepath.Join(root, "real-codex"))
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(data))
	if path == "" {
		return "", errors.New("recorded codex path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func resolveAccount(store *state.Store, reference string) (state.Account, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return state.Account{}, errors.New("an account ID or label is required")
	}
	accounts := store.Accounts()
	for _, account := range accounts {
		if account.ID == reference {
			return account, nil
		}
	}
	matches := make([]state.Account, 0, 2)
	lowered := strings.ToLower(reference)
	for _, account := range accounts {
		if strings.HasPrefix(strings.ToLower(account.Label), lowered) {
			matches = append(matches, account)
		}
	}
	switch len(matches) {
	case 0:
		return state.Account{}, fmt.Errorf("no account matches %q", reference)
	case 1:
		return matches[0], nil
	default:
		labels := make([]string, 0, len(matches))
		for _, account := range matches {
			labels = append(labels, fmt.Sprintf("%s (%s)", account.Label, account.ID))
		}
		return state.Account{}, fmt.Errorf("%q matches several accounts: %s", reference, strings.Join(labels, ", "))
	}
}

// parseInterleaved lets flags appear before or after positional arguments.
// Go's flag package stops at the first positional, which is the right rule for
// `run` (everything after it belongs to the Codex CLI) but surprising anywhere
// else, where a trailing --timeout would be silently ignored.
func parseInterleaved(flags *flag.FlagSet, args []string) ([]string, error) {
	positionals := make([]string, 0, len(args))
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		rest := flags.Args()
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		args = rest[1:]
	}
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func withTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
