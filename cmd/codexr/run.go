package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/mikaeww/codexr-windows/internal/mux"
	"github.com/mikaeww/codexr-windows/internal/state"
)

type decision struct {
	Account state.Account
	Reason  mux.RouteReason
	Sticky  bool
	Note    string
}

func commandPick(args []string) error {
	flags := flag.NewFlagSet("pick", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "emit machine-readable JSON")
	directory := flags.String("dir", "", "project directory to resolve (default: working directory)")
	fresh := flags.Bool("new", false, "ignore the stored project assignment")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}
	defer session.close()

	project, err := projectDirectory(*directory)
	if err != nil {
		return err
	}
	chosen, err := decide(ctx, session, project, *fresh, "")
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"accountId": chosen.Account.ID,
			"label":     chosen.Account.Label,
			"codexHome": chosen.Account.CodexHome,
			"sticky":    chosen.Sticky,
			"note":      chosen.Note,
			"reason":    chosen.Reason,
		})
	}
	fmt.Printf("%s (%s)\n", chosen.Account.Label, chosen.Account.ID)
	fmt.Printf("  home    %s\n", chosen.Account.CodexHome)
	fmt.Printf("  why     %s\n", chosen.Note)
	fmt.Printf("  detail  %s\n", describeReason(chosen.Reason))
	return nil
}

func commandRun(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	fresh := flags.Bool("new", false, "route this launch again instead of reusing the project's subscription")
	forced := flags.String("account", "", "force a specific account ID or label")
	dryRun := flags.Bool("dry-run", false, "print the resolved command without running it")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}

	project, err := projectDirectory("")
	if err != nil {
		session.close()
		return err
	}
	chosen, err := decide(ctx, session, project, *fresh, *forced)
	if err != nil {
		session.close()
		return err
	}

	environment := codexEnvironment(chosen.Account, session.store)
	command := append([]string{session.realCodex}, flags.Args()...)

	if *dryRun {
		session.close()
		fmt.Printf("CODEX_HOME=%s\n", chosen.Account.CodexHome)
		fmt.Printf("%s\n", strings.Join(command, " "))
		return nil
	}

	fmt.Fprintf(os.Stderr, "codexr: %s — %s\n", chosen.Account.Label, chosen.Note)

	shutdownCtx, cancelShutdown := withTimeout(context.Background(), 5*time.Second)
	session.multiplexer.Shutdown(shutdownCtx)
	cancelShutdown()

	return runProcess(session.realCodex, command[1:], environment)
}

func decide(ctx context.Context, session *session, project string, fresh bool, forced string) (decision, error) {
	if forced != "" {
		account, err := resolveAccount(session.store, forced)
		if err != nil {
			return decision{}, err
		}
		if err := session.store.SetProjectOwner(project, account.ID); err != nil {
			return decision{}, err
		}
		return decision{Account: account, Note: "forced with --account"}, nil
	}

	if !fresh {
		if ownerID, ok := session.store.ProjectOwner(project); ok {
			if account, exists := session.store.Account(ownerID); exists && account.Enabled {
				capacityCtx, cancel := withTimeout(ctx, 30*time.Second)
				hasCapacity := session.multiplexer.AccountHasCapacity(capacityCtx, ownerID)
				cancel()
				if hasCapacity {
					return decision{
						Account: account,
						Sticky:  true,
						Note:    "already assigned to this directory",
					}, nil
				}
			}
		}
	}

	pickCtx, cancel := withTimeout(ctx, 45*time.Second)
	defer cancel()
	account, reason, err := session.multiplexer.PickAccount(pickCtx, nil)
	if err != nil {
		if errors.Is(err, mux.ErrNoCapacity()) {
			return decision{}, errors.New(
				"every enabled subscription is depleted; wait for a reset or add another with `codexr add`")
		}
		return decision{}, err
	}
	if err := session.store.SetProjectOwner(project, account.ID); err != nil {
		return decision{}, err
	}
	note := "routed by quota urgency"
	if fresh {
		note = "re-routed by quota urgency"
	}
	return decision{Account: account, Reason: reason, Note: note}, nil
}

func describeReason(reason mux.RouteReason) string {
	parts := make([]string, 0, 4)
	if reason.WeeklyUsedPercent != nil {
		parts = append(parts, fmt.Sprintf("weekly %.0f%% used", *reason.WeeklyUsedPercent))
	}
	if reason.WeeklyResetsAt != nil {
		reset := time.Unix(*reason.WeeklyResetsAt, 0).In(time.Local)
		parts = append(parts, fmt.Sprintf("resets %s", reset.Format("Mon 15:04")))
	}
	if reason.UrgencyScore != nil {
		parts = append(parts, fmt.Sprintf("urgency %.4f", *reason.UrgencyScore))
	}
	if reason.BankedResetCount != nil && *reason.BankedResetCount > 0 {
		parts = append(parts, fmt.Sprintf("%d banked resets", *reason.BankedResetCount))
	}
	if len(parts) == 0 {
		return "no quota data"
	}
	return strings.Join(parts, ", ")
}

func projectDirectory(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return override, nil
	}
	working, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return working, nil
}

func codexEnvironment(account state.Account, store *state.Store) []string {
	overrides := map[string]string{
		"CODEX_HOME":                   account.CodexHome,
		"CODEX_SQLITE_HOME":            account.CodexHome,
		"CODEX_MUX_PRIMARY_CODEX_HOME": store.PrimaryCodexHome(),
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if runtime.GOOS == "windows" {
				name = strings.ToUpper(name)
			}
			if _, replaced := overrides[name]; replaced {
				continue
			}
		}
		environment = append(environment, entry)
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}
