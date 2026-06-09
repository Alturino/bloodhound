package cmd

import (
	"github.com/spf13/cobra"
)

var IDXWorker = &cobra.Command{
	Use:          "idx",
	Short:        "IDX announcement fetcher",
	SilenceUsage: true,
	Aliases:      []string{"ir"},
}

func init() {
	IDXWorker.AddCommand(IDXRunCmd, IDXRunAnnCmd, IDXRunAttCmd)
}
