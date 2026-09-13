package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mikaeww/codexr-windows/internal/mux"
)

func commandAccounts(args []string) error {
	flags := flag.NewFlagSet("accounts", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "emit machine-readable JSON")
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

	lookupCtx, cancelLookup := withTimeout(ctx, 45*time.Second)
	defer cancelLookup()
	snapshots := session.multiplexer.Accounts(lookupCtx)
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(snapshots)
	}
	printAccounts(snapshots, session.store.ProjectCounts())
	return nil
}

func printAccounts(snapshots []mux.AccountSnapshot, projects map[string]int) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tLABEL\tPLAN\t5H\tWEEKLY\tRESETS\tCHATS\tDIRS\tSTATE")
	for _, snapshot := range snapshots {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
			snapshot.ID,
			snapshot.Label,
			planColumn(snapshot),
			shortColumn(snapshot),
			weeklyColumn(snapshot),
			resetColumn(snapshot),
			snapshot.ThreadCount,
			projects[snapshot.ID],
			stateColumn(snapshot),
		)
	}
	_ = writer.Flush()
}

func shortColumn(snapshot mux.AccountSnapshot) string {
	short := snapshot.Short()
	if short == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%% used", short.UsedPercent)
}

func planColumn(snapshot mux.AccountSnapshot) string {
	if snapshot.PlanLabel != "" {
		return snapshot.PlanLabel
	}
	if snapshot.PlanType != "" {
		return snapshot.PlanType
	}
	return "—"
}

func weeklyColumn(snapshot mux.AccountSnapshot) string {
	weekly := snapshot.Weekly()
	if weekly == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%% used", weekly.UsedPercent)
}

func resetColumn(snapshot mux.AccountSnapshot) string {
	weekly := snapshot.Weekly()
	if weekly == nil || weekly.ResetsAt == nil {
		return "—"
	}
	reset := time.Unix(*weekly.ResetsAt, 0).In(time.Local)
	remaining := time.Until(reset)
	if remaining <= 0 {
		return "due"
	}
	return fmt.Sprintf("%s (%s)", reset.Format("Mon 15:04"), compactDuration(remaining))
}

func stateColumn(snapshot mux.AccountSnapshot) string {
	if snapshot.Error != "" {
		return "error: " + firstLine(snapshot.Error)
	}
	if !snapshot.Enabled {
		return "disabled"
	}
	if !snapshot.Connected {
		return "needs login"
	}
	if weekly := snapshot.Weekly(); weekly != nil && weekly.UsedPercent >= 100 {
		return "depleted"
	}
	if snapshot.Controller {
		return "ready (controller)"
	}
	return "ready"
}

func compactDuration(value time.Duration) string {
	hours := int(value.Hours())
	if hours >= 24 {
		return fmt.Sprintf("%dd%dh", hours/24, hours%24)
	}
	if hours >= 1 {
		return fmt.Sprintf("%dh%dm", hours, int(value.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(value.Minutes()))
}

func firstLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return value[:index]
	}
	return value
}

func commandAdd(args []string) error {
	if len(args) == 0 {
		return errors.New("a label is required, for example: codexr add Work")
	}
	label := strings.Join(args, " ")

	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}
	defer session.close()

	addCtx, cancelAdd := withTimeout(ctx, 30*time.Second)
	defer cancelAdd()
	account, err := session.multiplexer.AddAccount(addCtx, label)
	if err != nil {
		return err
	}
	fmt.Printf("Added %s (%s)\n", account.Label, account.ID)
	fmt.Printf("Sign in with: codexr login %s\n", account.ID)
	return nil
}

func commandLogin(args []string) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	timeout := flags.Duration("timeout", 10*time.Minute, "how long to wait for the browser sign-in")
	positionals, err := parseInterleaved(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) == 0 {
		return errors.New("an account ID or label is required")
	}

	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}
	defer session.close()

	account, err := resolveAccount(session.store, positionals[0])
	if err != nil {
		return err
	}

	startCtx, cancelStart := withTimeout(ctx, 30*time.Second)
	defer cancelStart()
	raw, err := session.multiplexer.StartLogin(startCtx, account.ID, "chatgptDeviceCode")
	if err != nil {
		return fmt.Errorf("start device-code login: %w", err)
	}
	var login struct {
		VerificationURL string `json:"verificationUrl"`
		UserCode        string `json:"userCode"`
		LoginID         string `json:"loginId"`
	}
	if err := json.Unmarshal(raw, &login); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	if login.VerificationURL == "" || login.UserCode == "" {
		return fmt.Errorf("app-server returned no device code: %s", string(raw))
	}

	fmt.Printf("\nSign in to %s\n\n", account.Label)
	fmt.Printf("  1. Open   %s\n", login.VerificationURL)
	fmt.Printf("  2. Enter  %s\n\n", login.UserCode)
	fmt.Printf("Use the ChatGPT account whose subscription should back %q.\n", account.Label)
	fmt.Println("Waiting for the sign-in to complete… (Ctrl-C aborts)")

	waitCtx, cancelWait := withTimeout(ctx, *timeout)
	defer cancelWait()
	snapshot, err := waitForLogin(waitCtx, session, account.ID)
	if err != nil {
		return err
	}
	fmt.Printf("\nConnected %s as %s (%s)\n", account.Label, snapshot.Email, planColumn(snapshot))
	return nil
}

func waitForLogin(ctx context.Context, session *session, accountID string) (mux.AccountSnapshot, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return mux.AccountSnapshot{}, errors.New("timed out waiting for the device-code sign-in")
		case <-ticker.C:
			pollCtx, cancel := withTimeout(context.Background(), 15*time.Second)
			snapshot, err := session.multiplexer.Account(pollCtx, accountID)
			cancel()
			if err != nil {
				continue
			}
			if snapshot.Connected {
				return snapshot, nil
			}
		}
	}
}

func commandLogout(args []string) error {
	if len(args) == 0 {
		return errors.New("an account ID or label is required")
	}
	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}
	defer session.close()

	account, err := resolveAccount(session.store, args[0])
	if err != nil {
		return err
	}
	logoutCtx, cancelLogout := withTimeout(ctx, 30*time.Second)
	defer cancelLogout()
	if err := session.multiplexer.Logout(logoutCtx, account.ID); err != nil {
		return err
	}
	fmt.Printf("Signed out of %s (%s)\n", account.Label, account.ID)
	return nil
}

func commandRename(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: codexr rename <account> <new label>")
	}
	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}
	defer session.close()

	account, err := resolveAccount(session.store, args[0])
	if err != nil {
		return err
	}
	label := strings.Join(args[1:], " ")
	renameCtx, cancelRename := withTimeout(ctx, 30*time.Second)
	defer cancelRename()
	updated, err := session.multiplexer.UpdateAccount(renameCtx, account.ID, &label, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Renamed %s to %s\n", account.Label, updated.Label)
	return nil
}

func commandSetEnabled(args []string, enabled bool) error {
	if len(args) == 0 {
		return errors.New("an account ID or label is required")
	}
	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}
	defer session.close()

	account, err := resolveAccount(session.store, args[0])
	if err != nil {
		return err
	}
	updateCtx, cancelUpdate := withTimeout(ctx, 30*time.Second)
	defer cancelUpdate()
	updated, err := session.multiplexer.UpdateAccount(updateCtx, account.ID, nil, &enabled)
	if err != nil {
		return err
	}
	if enabled {
		fmt.Printf("%s is now included in routing\n", updated.Label)
	} else {
		fmt.Printf("%s is now excluded from routing\n", updated.Label)
	}
	return nil
}
