package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/service"
)

func Track(
	ctx context.Context,
	repo *repository.HTTPRepository,
	pool int,
	downloadJobCh chan jobs.DownloadAttachmentArgs,
	resDownloadJobCh chan jobs.DownloadRes,
	stopDownloadCh chan struct{},
) *cobra.Command {
	var emiten, keyword string
	var page, pageSize int

	track := service.NewTrack(ctx, repo, pool, downloadJobCh, resDownloadJobCh, stopDownloadCh)
	trackCmd := &cobra.Command{
		Use:     "track",
		Short:   "get stock announcements",
		Aliases: []string{"t"},
		Example: "bloodhound track --emiten=AADI --keyword='laporan keuangan' --size=100 --page=0",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.ValidateArgs(args); err != nil {
				log.Fatalln(err.Error())
			}
			res, err := track.Track(cmd.Context(), emiten, keyword, page, pageSize)
			if err != nil {
				log.Fatalln(err.Error())
			}
			log.Println(res)
		},
	}
	trackCmd.Flags().StringVarP(&emiten, "emiten", "e", "", "specify the stock ticker")
	trackCmd.Flags().StringVarP(&keyword, "keyword", "k", "", "specify title of the document")
	trackCmd.Flags().IntVarP(&pageSize, "size", "s", 200, "specify how much each page sized")
	trackCmd.Flags().IntVarP(&page, "page", "p", 0, "specify page")
	return trackCmd
}
