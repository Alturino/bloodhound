package main

import (
	"context"
	"log"
	"time"

	"github.com/imroc/req/v3"
	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/internal"
)

func main() {
	ctx := context.Background()

	rootCmd := &cobra.Command{}

	httpClient := req.ImpersonateFirefox()
	track := internal.NewTrack(httpClient)
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
			track.Track(cmd.Context(), emiten, keyword, page, pageSize)
		},
	}
	trackCmd.Flags().StringVarP(&emiten, "emiten", "e", "", "specify the stock ticker")
	trackCmd.Flags().StringVarP(&keyword, "keyword", "k", "", "specify title of the document")
	trackCmd.Flags().IntVarP(&pageSize, "size", "s", 100, "specify how much each page sized")
	trackCmd.Flags().IntVarP(&page, "p", "p", 0, "specify page")

	var interval int
	guardCmd := &cobra.Command{
		Use:     "guard",
		Short:   "watch stock announcements",
		Aliases: []string{"g"},
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.ValidateArgs(args); err != nil {
				log.Fatalln(err.Error())
			}
			tick := time.Tick(time.Minute * time.Duration(interval))
			for range tick {
				track.Track(cmd.Context(), "", "", 0, 1000)
			}
		},
	}
	guardCmd.Flags().
		IntVarP(&interval, "interval", "i", 15, "specify the interval in minute to check for new announcements")

	commands := []*cobra.Command{trackCmd, guardCmd}
	rootCmd.AddCommand(commands...)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Fatalln(err.Error())
	}
}
