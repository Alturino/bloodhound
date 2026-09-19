package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	mock    bool
)

var RootCmd = &cobra.Command{
	Use:     "bloodhound",
	Aliases: []string{"bd"},
	Short:   "Bloodhound worker service",
	Long:    `IDX announcement fetcher and Stockbit integration worker`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if mock {
			viper.Set("app.idx.mock_mode", true)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(IDXWorker, StockbitWorker)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "bloodhound.yaml", "config file path")
	RootCmd.PersistentFlags().BoolVar(&mock, "mock", false, "enable IDX mock mode")
	if err := viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config")); err != nil {
		log.Fatal(err)
	}
	if err := viper.BindPFlag("mock", RootCmd.PersistentFlags().Lookup("mock")); err != nil {
		log.Fatal(err)
	}
}
