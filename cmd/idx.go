package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var IDXWorker = &cobra.Command{
	Use:          "idx",
	Short:        "IDX announcement fetcher",
	SilenceUsage: true,
	Aliases:      []string{"i"},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cmds := strings.Split(cmd.CommandPath(), " ")
		name := strings.Join(cmds, "_")
		viper.Set("app.name", name)
	},
}

func init() {
	IDXWorker.AddCommand(IDXRunCmd, IDXRunAnnCmd, IDXRunAttCmd)
}
