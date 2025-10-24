package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/service"
)

func TrackTillEmpty(
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
	tillEmptyCmd := &cobra.Command{
		Use:     "track-empty",
		Short:   "get all stock announcements",
		Aliases: []string{"te"},
		Example: "bloodhound track-empty --emiten=AADI --keyword='laporan keuangan' --size=100 --page=0",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.ValidateArgs(args); err != nil {
				log.Fatalln(err.Error())
			}
			track.TrackTillEmpty(cmd.Context(), emiten, keyword, page, pageSize)
		},
	}
	tillEmptyCmd.Flags().StringVarP(&emiten, "emiten", "e", "", "specify the stock ticker")
	tillEmptyCmd.Flags().StringVarP(&keyword, "keyword", "k", "", "specify title of the document")
	tillEmptyCmd.Flags().IntVarP(&pageSize, "size", "s", 200, "specify how much each page sized")
	tillEmptyCmd.Flags().IntVarP(&page, "page", "p", 0, "specify page")
	return tillEmptyCmd
}
