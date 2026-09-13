package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/mikaeww/codexr-windows/internal/desktop"
)

func commandDesktop(args []string) error {
	flags := flag.NewFlagSet("desktop", flag.ContinueOnError)
	list := flags.Bool("list", false, "list running Codex Desktop instances and exit")
	separate := flags.Bool("new-window", false, "open another window instead of switching the running one")
	stop := flags.Bool("stop", false, "close every running Codex Desktop instance and exit")
	positionals, err := parseInterleaved(flags, args)
	if err != nil {
		return err
	}

	if *list {
		return listDesktopInstances()
	}
	if *stop {
		return stopDesktopInstances()
	}
	if len(positionals) == 0 {
		return errors.New("an account ID or label is required, or pass --list")
	}

	store, err := openStore()
	if err != nil {
		return err
	}
	account, err := resolveAccount(store, positionals[0])
	if err != nil {
		return err
	}

	if *separate {
		if err := desktop.Launch(account.CodexHome, len(desktop.Running()) > 0); err != nil {
			return err
		}
		fmt.Printf("Opening another Codex Desktop window as %s\n", account.Label)
		return nil
	}

	if _, running := desktop.RunningFor(account.CodexHome); running {
		fmt.Printf("Codex Desktop is already running as %s\n", account.Label)
		return nil
	}
	if len(desktop.Running()) == 0 {
		if err := desktop.Launch(account.CodexHome, false); err != nil {
			return err
		}
		fmt.Printf("Opening Codex Desktop as %s\n", account.Label)
		return nil
	}
	fmt.Printf("Restarting Codex Desktop as %s…\n", account.Label)
	if err := desktop.Restart(account.CodexHome); err != nil {
		return err
	}
	fmt.Printf("Codex Desktop is now running as %s\n", account.Label)
	return nil
}

func stopDesktopInstances() error {
	instances := desktop.Running()
	if len(instances) == 0 {
		fmt.Println("No Codex Desktop instance is running.")
		return nil
	}
	for _, instance := range instances {
		if err := desktop.Stop(instance); err != nil {
			return err
		}
		fmt.Printf("Closed the window using %s\n", instance.CodexHome)
	}
	return nil
}

func listDesktopInstances() error {
	instances := desktop.Running()
	if len(instances) == 0 {
		fmt.Println("No Codex Desktop instance is running.")
		return nil
	}
	store, storeErr := openStore()
	labels := map[string]string{}
	if storeErr == nil {
		for _, account := range store.Accounts() {
			labels[filepath.Clean(account.CodexHome)] = account.Label
		}
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "PID\tPORT\tSUBSCRIPTION\tCODEX_HOME")
	for _, instance := range instances {
		label := labels[instance.CodexHome]
		if label == "" {
			label = "unknown"
		}
		port := instance.Port
		if port == "" {
			port = "—"
		}
		fmt.Fprintf(writer, "%d\t%s\t%s\t%s\n", instance.PID, port, label, instance.CodexHome)
	}
	return writer.Flush()
}
