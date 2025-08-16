package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
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
					err := track.repo.DownloadFile(ctx, job.Emiten, job.Attachment)
					if err != nil {
						log.Println("workerID", workerID, "failed to download file:", err.Error())
						resCh <- jobs.DownloadRes{Err: err, URL: job.FullSavePath, AttachmentID: job.ID, WorkerID: workerID}
						continue
					}
					log.Println("workerID", workerID, "successfully downloaded file")
					resCh <- jobs.DownloadRes{Err: nil, URL: job.FullSavePath, AttachmentID: job.ID, WorkerID: workerID}
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
	responses := make([]response.Response, 0, 100)
	for {
		res, err := t.repo.Get(ctx, emiten, keyword, currentPage, pageSize)
		if err != nil {
			log.Println("failed to get announcement:", err.Error())
			continue
		}
		if len(res.Replies) == 0 {
			log.Println("Replies is empty, stopping")
			break
		}
		responses = append(responses, res)
		log.Println("successfully appending to responses")
		currentPage++
		time.Sleep(3 * time.Second)
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
) (response.Response, error) {
	res, err := t.repo.Get(ctx, emiten, keyword, page, pageSize)
	if err != nil {
		err = fmt.Errorf("Track failed with error: %w", err)
		return response.Response{}, err
	}
	return res, nil
}

func (t Track) Download(ctx context.Context, responses []response.Response) {
	var wg sync.WaitGroup
	for _, response := range responses {
		for _, reply := range response.Replies {
			for _, attachment := range reply.Attachments {
				wg.Add(1)
				go func() {
					defer wg.Done()
					job := jobs.DownloadJob{
						Emiten:     reply.Pengumuman.KodeEmiten,
						Attachment: attachment,
					}
					log.Println("sending download job", job)
					t.downloadJobCh <- job
				}()
				wg.Wait()
			}
		}
	}
}
