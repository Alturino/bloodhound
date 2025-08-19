package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/repository"
	"github.com/Alturino/bloodhound/internal/response"
	"github.com/Alturino/bloodhound/internal/worker"
)

type Track struct {
	repo             repository.HTTPRepository
	pool             int
	downloadJobCh    chan jobs.DownloadJob
	resDownloadJobCh chan jobs.DownloadRes
	stopDownloadCh   chan struct{}
}

func NewTrack(ctx context.Context, repository repository.HTTPRepository, pool int) Track {
	downloadJobCh := make(chan jobs.DownloadJob, pool)
	resDownloadJobCh := make(chan jobs.DownloadRes, pool)
	stopDownloadCh := make(chan struct{}, pool)
	track := Track{
		repo:             repository,
		pool:             pool,
		downloadJobCh:    downloadJobCh,
		resDownloadJobCh: resDownloadJobCh,
		stopDownloadCh:   stopDownloadCh,
	}

	go worker.WorkerPool(
		ctx,
		pool,
		track.downloadJobCh,
		track.resDownloadJobCh,
		track.stopDownloadCh,
		func(ctx context.Context, workerID int, jobCh <-chan jobs.DownloadJob, resCh chan<- jobs.DownloadRes, stopCh <-chan struct{}) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			defer close(resCh)
			for {
				select {
				case <-stopCh:
					log.Println("workerID", workerID, "received stop signal, stopping worker")
					return
				case <-ctx.Done():
					log.Println("workerID", workerID, "received context done, stopping worker")
					return
				case job, ok := <-jobCh:
					if !ok {
						log.Println(
							"workerID", workerID,
							"channel is closed stop receiving from channel",
						)
						return
					}
					log.Println("workerID", workerID, "received job:", job)
					var wg sync.WaitGroup
					for _, attachment := range job.Attacments {
						if !strings.HasSuffix(attachment.OriginalFilename, ".pdf") {
							continue
						}
						wg.Add(1)
						go func(wg *sync.WaitGroup) {
							defer wg.Done()
							err := track.repo.DownloadFile(ctx, job.Emiten, attachment)
							if err != nil {
								log.Println(
									"workerID", workerID,
									"failed to download file:", err.Error(),
								)
								return
							}
							log.Println(
								"workerID", workerID,
								"successfully downloaded file", attachment.OriginalFilename,
							)
						}(&wg)
					}
					wg.Wait()
				}
			}
		},
	)
	return track
}

func (t Track) TrackTillEmpty(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	currentPage := page
	responses := make([]response.IdxResponse, 0, 100)
	for {
		res, err := t.repo.Get(ctx, emiten, keyword, currentPage, pageSize)
		currentPage++
		if err != nil {
			log.Println("failed to get announcement:", err.Error())
			continue
		}
		if len(res.Replies) == 0 {
			log.Println("Replies is empty, stopping")
			break
		}
		for _, reply := range res.Replies {
			emiten := strings.TrimSpace(reply.Pengumuman.KodeEmiten)
			log.Println("emiten:", emiten)
			t.downloadJobCh <- jobs.DownloadJob{Emiten: emiten, Attacments: reply.Attachments}
		}
		responses = append(responses, res)
		log.Println("successfully appending to responses")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := path.Join(homeDir, "Downloads", "responses")
	if err = os.MkdirAll(dir, os.FileMode(0o755)); err != nil {
		return err
	}

	timestamp := time.Now().Format("2006-01-02_15:04:05")
	filename := fmt.Sprintf("%s_responses.json", timestamp)
	filePath := filepath.Join(dir, filename)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(0o755))
	if err != nil {
		return err
	}

	if err = json.NewEncoder(file).Encode(responses); err != nil {
		return err
	}

	return nil
}

func (t Track) Track(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) (response.IdxResponse, error) {
	res, err := t.repo.Get(ctx, emiten, keyword, page, pageSize)
	if err != nil {
		err = fmt.Errorf("Track failed with error: %w", err)
		return response.IdxResponse{}, err
	}
	return res, nil
}
