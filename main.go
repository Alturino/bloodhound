package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alturino/bloodhound/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.RootCmd.ExecuteContext(ctx); err != nil {
		slog.ErrorContext(ctx, err.Error())
		os.Exit(1)
	}
}
