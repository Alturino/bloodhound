package main

import (
	"context"
	"log"
	"os"
	"path"
	"time"

	req "github.com/imroc/req/v3"
	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/internal"
	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/middleware"
	"github.com/Alturino/bloodhound/internal/repository"
)

func main() {
	ctx := context.Background()

	rootCmd := &cobra.Command{}

	homeDir := common.GetHomeDir()

	bloodhoundDir := path.Join(homeDir, "Downloads", "bloodhound")
	if err := os.MkdirAll(bloodhoundDir, os.FileMode(0o755)); err != nil {
		log.Fatalln(err.Error())
	}

	httpClient := req.ImpersonateChrome().
		// EnableDumpAll().
		// EnableTraceAll().
		// DisableKeepAlives().
		EnableAutoDecompress().
		AddCommonRetryCondition(middleware.ShouldGetCookie()).
		SetCommonRetryCount(2).
		SetCommonRetryHook(middleware.GetCookie(ctx)).
		SetCommonHeaders(map[string]string{
			"Connection":         "keep-alive",
			"Accept-Encoding":    "gzip",
			"Host":               "idx.co.id",
			"Referer":            "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/",
			"Sec-Fetch-Dest":     "document",
			"Sec-Fetch-Mode":     "navigate",
			"Sec-Fetch-Site":     "cross-site",
			"sec-ch-ua-platform": `"Android"`,
		}).
		SetOutputDirectory(bloodhoundDir).
		SetUserAgent("Mozilla/5.0 (Linux; Android 10; SM-A205U) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Mobile Safari/537.36").
		DisableAutoReadResponse()

	pool := 10
	repo := repository.NewHTTPRepository(httpClient)
	track := internal.NewTrack(ctx, repo, pool)
	var emiten, keyword string
	var page, pageSize int
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

	trackTillEmptyCmd := &cobra.Command{
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
	trackTillEmptyCmd.Flags().StringVarP(&emiten, "emiten", "e", "", "specify the stock ticker")
	trackTillEmptyCmd.Flags().
		StringVarP(&keyword, "keyword", "k", "", "specify title of the document")
	trackTillEmptyCmd.Flags().
		IntVarP(&pageSize, "size", "s", 200, "specify how much each page sized")
	trackTillEmptyCmd.Flags().IntVarP(&page, "page", "p", 0, "specify page")

	var interval int
	guardCmd := &cobra.Command{
		Use:     "watchdog",
		Short:   "watch stock announcements",
		Aliases: []string{"w"},
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

	commands := []*cobra.Command{trackCmd, trackTillEmptyCmd, guardCmd}
	rootCmd.AddCommand(commands...)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Fatalln(err.Error())
	}
}
