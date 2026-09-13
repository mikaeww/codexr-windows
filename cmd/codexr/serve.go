package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mikaeww/codexr-windows/internal/control"
)

const defaultControlPort = 48123

func commandServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := flags.Int("port", 0, "control API port (default 48123)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, cancel := signalContext()
	defer cancel()
	session, err := openSession(ctx, os.Stderr)
	if err != nil {
		return err
	}
	defer session.close()

	token, err := loadOrCreateToken(session.store.Root())
	if err != nil {
		return err
	}
	listenPort := resolvePort(*port)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort))
	if err != nil {
		return fmt.Errorf("bind control API: %w", err)
	}
	server := control.New(listener.Addr().String(), token, session.multiplexer, false)

	fmt.Printf("Control API on http://%s\n", listener.Addr().String())
	fmt.Printf("Token file  %s\n", filepath.Join(session.store.Root(), "control-token"))
	fmt.Printf("Example     curl -H \"X-Codex-Mux-Token: $(cat %s)\" http://%s/v1/accounts\n",
		filepath.Join(session.store.Root(), "control-token"), listener.Addr().String())

	errs := make(chan error, 1)
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errs <- serveErr
			return
		}
		errs <- nil
	}()

	select {
	case <-ctx.Done():
	case err := <-errs:
		if err != nil {
			return err
		}
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	return server.Shutdown(shutdownCtx)
}

func resolvePort(override int) int {
	if override > 0 && override <= 65535 {
		return override
	}
	if value := os.Getenv("CODEX_MUX_CONTROL_PORT"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 65535 {
			return parsed
		}
	}
	return defaultControlPort
}

func loadOrCreateToken(root string) (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CODEX_MUX_CONTROL_TOKEN")); configured != "" {
		return validateToken(configured)
	}
	path := filepath.Join(root, "control-token")
	if data, err := os.ReadFile(path); err == nil {
		token, validateErr := validateToken(string(data))
		if validateErr != nil {
			return "", fmt.Errorf("read control token: %w", validateErr)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return "", fmt.Errorf("secure control token: %w", err)
		}
		return token, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read control token: %w", err)
	}
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate control token: %w", err)
	}
	token := hex.EncodeToString(buffer)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", fmt.Errorf("write control token: %w", err)
	}
	return token, nil
}

func validateToken(value string) (string, error) {
	token := strings.TrimSpace(value)
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("control token must be exactly 32 random bytes encoded as hexadecimal")
	}
	return token, nil
}

func commandDoctor(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("doctor takes no arguments")
	}
	failures := 0
	report := func(ok bool, name, detail string) {
		mark := "ok  "
		if !ok {
			mark = "FAIL"
			failures++
		}
		fmt.Printf("[%s] %-22s %s\n", mark, name, detail)
	}

	realCodex, err := resolveRealCodex()
	if err != nil {
		report(false, "codex binary", err.Error())
	} else {
		report(true, "codex binary", realCodex)
		output, versionErr := exec.Command(realCodex, "--version").CombinedOutput()
		if versionErr != nil {
			report(false, "codex version", versionErr.Error())
		} else {
			report(true, "codex version", strings.TrimSpace(string(output)))
		}
		help, helpErr := exec.Command(realCodex, "app-server", "--help").CombinedOutput()
		report(helpErr == nil && strings.Contains(string(help), "app server"),
			"app-server support", "codex app-server responds")
	}

	store, err := openStore()
	if err != nil {
		report(false, "state store", err.Error())
		fmt.Printf("\n%d check(s) failed\n", failures)
		return errors.New("doctor found problems")
	}
	report(true, "state store", store.Root())
	report(true, "diagnostics log", diagnosticsLogPath(store.Root()))
	report(true, "primary codex home", store.PrimaryCodexHome())

	accounts := store.Accounts()
	enabled := 0
	for _, account := range accounts {
		if account.Enabled {
			enabled++
		}
	}
	report(len(accounts) > 0, "accounts", fmt.Sprintf("%d configured, %d enabled", len(accounts), enabled))

	port := resolvePort(0)
	listener, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if listenErr == nil {
		_ = listener.Close()
		report(true, "control port", fmt.Sprintf("%d is free", port))
	} else {
		report(true, "control port", fmt.Sprintf("%d is in use (serve already running?)", port))
	}

	if failures > 0 {
		fmt.Printf("\n%d check(s) failed\n", failures)
		return errors.New("doctor found problems")
	}
	fmt.Println("\nAll checks passed")
	return nil
}
