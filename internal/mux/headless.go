package mux

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/mikaeww/codexr-windows/internal/protocol"
	"github.com/mikaeww/codexr-windows/internal/state"
)

type HeadlessOptions struct {
	RealExecutable string
	Store          *state.Store
	ClientName     string
	ClientVersion  string
	// Diagnostics collects child app-server logs. Point it at a file, never at
	// the terminal, whenever a full-screen UI is on screen — the children log
	// on their own schedule and will otherwise draw straight over it.
	Diagnostics io.Writer
}

// Headless boots the account pool without owning stdin or stdout. The stdio
// multiplexer answers `initialize` on behalf of a connected app-server client;
// a command-line caller has no such client and must perform the handshake
// itself before any account method will answer.
func Headless(ctx context.Context, options HeadlessOptions) (*Multiplexer, error) {
	multiplexer, err := New(Options{
		RealExecutable: options.RealExecutable,
		RealArgs:       []string{"app-server"},
		Environment:    os.Environ(),
		Store:          options.Store,
		Output:         io.Discard,
		Diagnostics:    options.Diagnostics,
	})
	if err != nil {
		return nil, err
	}
	if err := multiplexer.Start(ctx); err != nil {
		return nil, err
	}
	if err := multiplexer.Initialize(ctx, options.ClientName, options.ClientVersion); err != nil {
		multiplexer.Close()
		return nil, err
	}
	return multiplexer, nil
}

// Initialize performs the app-server handshake against every started child.
func (m *Multiplexer) Initialize(ctx context.Context, clientName, clientVersion string) error {
	params, err := json.Marshal(map[string]any{
		"clientInfo": map[string]any{"name": clientName, "version": clientVersion},
	})
	if err != nil {
		return fmt.Errorf("encode initialize params: %w", err)
	}
	m.initializationMu.Lock()
	m.initializeParams = params
	m.initialized = true
	m.initializationMu.Unlock()

	var firstErr error
	initialized := 0
	for _, entry := range m.childEntries() {
		requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		_, err := entry.child.Request(requestCtx, "initialize", params)
		cancel()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("initialize %s: %w", entry.account.Label, err)
			}
			continue
		}
		_ = entry.child.Send(protocol.Message{Method: "initialized"})
		initialized++
	}
	if initialized == 0 {
		return fmt.Errorf("no subscription completed the app-server handshake: %w", firstErr)
	}
	return nil
}

// PickAccount returns the subscription the routing algorithm would assign a new
// conversation to, together with the inputs that produced the decision.
func (m *Multiplexer) PickAccount(ctx context.Context, excluded map[string]struct{}) (state.Account, RouteReason, error) {
	return m.chooseAccountExcluding(ctx, excluded)
}

// Weekly returns the longest rate-limit window, which is the one the routing
// algorithm scores on. Short returns the shortest window, used only to break
// ties between accounts with equal urgency.
func (s AccountSnapshot) Weekly() *RateLimitWindow {
	weekly, _ := longestAndShortestWindow(s.RateLimits)
	return weekly
}

func (s AccountSnapshot) Short() *RateLimitWindow {
	_, short := longestAndShortestWindow(s.RateLimits)
	return short
}

// Account returns one subscription snapshot without fanning out across the pool.
func (m *Multiplexer) Account(ctx context.Context, accountID string) (AccountSnapshot, error) {
	return m.accountSnapshotWithProfile(ctx, accountID, false)
}

// AccountHasCapacity reports whether an account can still take a turn.
func (m *Multiplexer) AccountHasCapacity(ctx context.Context, accountID string) bool {
	snapshot, err := m.accountSnapshotWithProfile(ctx, accountID, false)
	if err != nil {
		return false
	}
	return accountHasCapacity(snapshot)
}

// Shutdown stops every child and waits for it to exit.
func (m *Multiplexer) Shutdown(ctx context.Context) {
	entries := m.childEntries()
	for _, entry := range entries {
		_ = entry.child.Close()
	}
	for _, entry := range entries {
		select {
		case <-entry.child.Done():
		case <-ctx.Done():
			return
		}
	}
}

// Store exposes the routing metadata store to command-line callers.
func (m *Multiplexer) Store() *state.Store {
	return m.store
}

// ErrNoCapacity reports that every enabled subscription is depleted.
func ErrNoCapacity() error {
	return errNoSubscriptionCapacity
}
