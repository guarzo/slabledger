// showprep-transition is local, explicit one-time consent tooling. It neither
// migrates the application schema nor starts evidence collection/schedulers.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/observability"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

func main() {
	if err := runWithContext(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func runWithContext() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return run(ctx, os.Args[1:], os.Stdout)
}
func run(ctx context.Context, args []string, out io.Writer) error {
	o, err := parse(args)
	if err != nil {
		return err
	}
	d, err := loadSnapshot(o.snapshot, o.operation)
	if err != nil {
		return err
	}
	// No .env loader or URL flag: the operator explicitly supplies DATABASE_URL
	// in the process environment. Open's retry logger is muted to avoid secrets.
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL environment variable is required")
	}
	db, err := postgres.Open(ctx, url, observability.NewNoopLogger())
	if err != nil {
		return errors.New("database connection failed; verify DATABASE_URL and connectivity")
	}
	defer func() { _ = db.Close() }()
	store := postgres.NewShowPrepStore(db.DB)
	return execute(ctx, o, d, store, sp.NewService(store, nil, nil), out)
}
