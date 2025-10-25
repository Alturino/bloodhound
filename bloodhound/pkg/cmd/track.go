package cmd

import (
	"log"

	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/internal/client"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/service"
)

func Track() *cobra.Command {
	var emiten, keyword string
	var page, pageSize int

	pool := 10
	downloadCh := make(chan jobs.DownloadAttachmentArgs, pool)
	defer close(downloadCh)

	resDownloadCh := make(chan jobs.DownloadRes, pool)
	defer close(resDownloadCh)

	stopDownloadCh := make(chan struct{}, pool)
	defer close(stopDownloadCh)

	cmd := &cobra.Command{
		Use:     "track",
		Short:   "get stock announcements",
		Aliases: []string{"t"},
		Example: "bloodhound track --emiten=AADI --keyword='laporan keuangan' --size=100 --page=0",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.ValidateArgs(args); err != nil {
				log.Fatalln(err.Error())
			}

			repo := repository.NewHTTPRepository(client.NewHTTPClient(cmd.Context()))
			track := service.NewTrack(
				cmd.Context(),
				repo,
				pool,
				downloadCh,
				resDownloadCh,
				stopDownloadCh,
			)
			res, err := track.Track(cmd.Context(), emiten, keyword, page, pageSize)
			if err != nil {
				log.Fatalln(err.Error())
			}
			log.Println(res)
		},
	}
	cmd.Flags().StringVarP(&emiten, "emiten", "e", "", "specify the stock ticker")
	cmd.Flags().StringVarP(&keyword, "keyword", "k", "", "specify title of the document")
	cmd.Flags().IntVarP(&pageSize, "size", "s", 200, "specify how much each page sized")
	cmd.Flags().IntVarP(&page, "page", "p", 0, "specify page")
	return cmd
}
