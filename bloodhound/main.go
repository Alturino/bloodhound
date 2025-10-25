package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/pkg/cmd"
)

func main() {
	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()

	ctx := context.Background()
	logger := log.Logger.With().Logger()

	logger.Debug().Msg("adding listener sigint and sigterm")
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	logger.Debug().Msg("added listener sigint and sigterm")

	rootCmd := &cobra.Command{}

	trackCmd := cmd.Track()
	tillEmptyCmd := cmd.TrackTillEmpty()

	commands := []*cobra.Command{trackCmd, tillEmptyCmd}
	rootCmd.AddCommand(commands...)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		err = fmt.Errorf("failed to execute bloodhound command: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}
}
