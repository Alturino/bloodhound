package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/internal"
)

func main() {
	ctx := context.Background()

	rootCmd := &cobra.Command{}

	var emiten, keyword string
	var page, pageSize int
	trackCmd := &cobra.Command{
		Use:     "track",
		Short:   "get stock announcements",
		Aliases: []string{"t"},
		Example: "bloodhound track --emiten=AADI --keyword='laporan keuangan' --page-size=100 --page=0",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.ValidateArgs(args); err != nil {
				log.Fatalln(err.Error())
			}
			internal.Track(cmd.Context(), emiten, keyword, page, pageSize)
		},
	}
	trackCmd.Flags().StringVarP(&emiten, "emiten", "e", "", "specify the stock ticker")
	trackCmd.Flags().StringVarP(&keyword, "keyword", "k", "", "specify title of the document")
	trackCmd.Flags().IntVarP(&pageSize, "size", "s", 100, "specify how much each page sized")
	trackCmd.Flags().IntVarP(&page, "p", "p", 0, "specify page")

	commands := []*cobra.Command{
		trackCmd,
		{
			Use:        "guard",
			Short:      "watch stock announcements",
			Aliases:    []string{"g"},
			ArgAliases: []string{""},
			Run: func(cmd *cobra.Command, args []string) {
				if err := cmd.ValidateArgs(args); err != nil {
					log.Fatalln(err.Error())
				}
			},
		},
	}
	rootCmd.AddCommand(commands...)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Fatalln(err.Error())
	}
}
