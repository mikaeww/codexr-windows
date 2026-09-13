package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mikaeww/codexr-windows/internal/tui"
)

func commandTUI(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("the account switcher takes no arguments")
	}

	ctx, cancel := signalContext()
	defer cancel()

	fmt.Fprintln(os.Stderr, "Easy-G — starting subscriptions…")
	session, err := openSession(ctx, nil)
	if err != nil {
		return err
	}

	project, err := projectDirectory("")
	if err != nil {
		session.close()
		return err
	}
	projects, err := loadElectronProjects(session.store.PrimaryCodexHome())
	if err != nil {
		session.close()
		return err
	}

	result, err := tui.Run(ctx, tui.Options{
		Multiplexer: session.multiplexer,
		Store:       session.store,
		Project:     project,
		Projects:    projects,
		Version:     version,
	})
	if err != nil {
		session.close()
		return err
	}
	if result.Launch == nil {
		session.close()
		return nil
	}
	if result.Project != "" {
		if err := os.Chdir(result.Project); err != nil {
			session.close()
			return fmt.Errorf("change to project directory: %w", err)
		}
	}

	environment := codexEnvironment(*result.Launch, session.store)
	shutdownCtx, cancelShutdown := withTimeout(context.Background(), 5*time.Second)
	session.multiplexer.Shutdown(shutdownCtx)
	cancelShutdown()

	fmt.Fprintf(os.Stderr, "Easy-G: starting Codex on %s\n", result.Launch.Label)
	return runProcess(session.realCodex, nil, environment)
}
