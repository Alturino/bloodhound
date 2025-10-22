package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/downloader"
)

func Downloader(ctx context.Context) *cobra.Command {
	var interval time.Duration
	downloaderCmd := &cobra.Command{
		Use:     "downloader",
		Short:   "worker to download files",
		Aliases: []string{"d"},
		Example: "bloodhound downloader -i 5s",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.ValidateArgs(args); err != nil {
				err = fmt.Errorf("argument validation failed: %w", err)
				log.Fatalln(err.Error())
			}
			downloader.StartDownloader(ctx)
		},
	}
	downloaderCmd.Flags().
		DurationVarP(&interval, "interval", "i", time.Second*5, "delay interval before worker can download the file again")
	return downloaderCmd
}
