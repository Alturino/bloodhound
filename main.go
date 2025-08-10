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
	commands := []*cobra.Command{
		{
			Use:     "track",
			Short:   "track stock announcements",
			Aliases: []string{"t"},
			Run: func(cmd *cobra.Command, args []string) {
				internal.Track(cmd.Context())
			},
		},
	}
	rootCmd.AddCommand(commands...)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Fatalln(err.Error())
	}
}
