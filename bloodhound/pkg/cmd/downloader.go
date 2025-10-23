package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/downloader"
)

func Downloader() *cobra.Command {
	downloaderCmd := &cobra.Command{
		Use:     "downloader",
		Short:   "worker to download files",
		Aliases: []string{"d"},
		Example: "bloodhound downloader",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.ValidateArgs(args); err != nil {
				err = fmt.Errorf("argument validation failed: %w", err)
				log.Fatalln(err.Error())
			}
			downloader.StartDownloader(cmd.Context())
		},
	}
	return downloaderCmd
}
