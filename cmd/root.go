package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	mock    bool
)

var RootCmd = &cobra.Command{
	Use:   "bloodhound",
	Short: "Bloodhound worker service",
	Long:  `IDX announcement fetcher and Stockbit integration worker`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if mock {
			viper.Set("app.idx.mock_mode", true)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(ServeCmd)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "bloodhound.yaml", "config file path")
	RootCmd.PersistentFlags().BoolVar(&mock, "mock", false, "enable IDX mock mode")
	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("mock", RootCmd.PersistentFlags().Lookup("mock"))
}
