package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/imroc/req/v3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/Alturino/bloodhound/internal/client"
	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/middleware"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/pkg/cmd"
)

func main() {
	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()

	ctx := context.Background()
	logger := log.Logger.With().Logger()

	logger.Debug().Msg("adding listener sigint and sigterm")
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	logger.Info().Msg("added listener sigint and sigterm")

	rootCmd := &cobra.Command{}

	homeDir := common.InitDir()
	defer os.RemoveAll(common.TempDir)

	bloodhoundDir := path.Join(homeDir, "Downloads", "bloodhound")
	if err := os.MkdirAll(bloodhoundDir, os.FileMode(0o755)); err != nil {
		err = fmt.Errorf("failed to create bloodhound download directory: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
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
			"Sec-Ch-Ua":          `"Chromium";v="139", "Not;A=Brand";v="99"`,
			"Sec-Fetch-Mode":     "navigate",
			"Sec-Fetch-Site":     "none",
			"sec-ch-ua-platform": `"Linux"`,
		}).
		SetOutputDirectory(bloodhoundDir).
		SetUserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36").
		DisableAutoReadResponse()

	repo := repository.NewHTTPRepository(client.HTTPClient{Client: httpClient})

	pool := 10

	downloadCh := make(chan jobs.DownloadAttachmentArgs, pool)
	defer close(downloadCh)

	resDownloadCh := make(chan jobs.DownloadRes, pool)
	defer close(resDownloadCh)

	stopDownloadCh := make(chan struct{}, pool)
	defer close(stopDownloadCh)

	trackCmd := cmd.Track(ctx, repo, pool, downloadCh, resDownloadCh, stopDownloadCh)
	tillEmptyCmd := cmd.TrackTillEmpty(ctx, repo, pool, downloadCh, resDownloadCh, stopDownloadCh)

	commands := []*cobra.Command{trackCmd, tillEmptyCmd}
	rootCmd.AddCommand(commands...)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		err = fmt.Errorf("failed to execute bloodhound command: %w", err)
		logger.Fatal().Err(err).Msg(err.Error())
	}
}
