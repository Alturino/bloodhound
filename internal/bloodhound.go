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
	repo                 repository.HTTPRepository
	pool                 int
	downloadJobCh        chan jobs.DownloadJob
	resDownloadJobCh     chan jobs.DownloadRes
	stopDownloadCh       chan struct{}
	getAnnouncementCh    chan jobs.FetchAnnouncementJob
	resGetAnnouncementCh chan jobs.FetchAnnouncementRes
	stopAnnouncementCh   chan struct{}
}

func NewTrack(ctx context.Context, repository repository.HTTPRepository, pool int) Track {
	downloadJobCh := make(chan jobs.DownloadJob, pool)
	resDownloadJobCh := make(chan jobs.DownloadRes, pool)
	stopDownloadCh := make(chan struct{}, pool)
	getAnnouncementCh := make(chan jobs.FetchAnnouncementJob, pool)
	resGetAnnouncementCh := make(chan jobs.FetchAnnouncementRes, pool)
	stopAnnouncementCh := make(chan struct{}, pool)
	track := Track{
		repo:                 repository,
		pool:                 pool,
		downloadJobCh:        downloadJobCh,
		resDownloadJobCh:     resDownloadJobCh,
		stopDownloadCh:       stopDownloadCh,
		getAnnouncementCh:    getAnnouncementCh,
		resGetAnnouncementCh: resGetAnnouncementCh,
		stopAnnouncementCh:   stopAnnouncementCh,
	}

	go worker.WorkerPool(
		ctx,
		pool,
		track.getAnnouncementCh,
		track.resGetAnnouncementCh,
		track.stopAnnouncementCh,
		func(ctx context.Context, workerID int, jobCh <-chan jobs.FetchAnnouncementJob, resCh chan<- jobs.FetchAnnouncementRes, stopCh <-chan struct{}) {
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
						log.Println("channel is closed stop receiving from channel")
						return
					}
					log.Println("workerID", workerID, "received job:", job)
					res, err := track.repo.Get(ctx, job.Emiten, job.Keyword, job.Page, job.PageSize)
					if err != nil {
						log.Println(
							"workerID", workerID,
							"failed to get announcement:", err.Error(),
						)
						resCh <- jobs.FetchAnnouncementRes{Err: err, Response: response.Response{}}
						continue
					}
					resCh <- jobs.FetchAnnouncementRes{Err: nil, Response: res}
				}
			}
		},
	)

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
	defer close(t.downloadJobCh)
	defer close(t.stopAnnouncementCh)
	defer close(t.stopDownloadCh)

	go func(ctx context.Context, page int) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer close(t.getAnnouncementCh)

		currentPage := page
		for {
			select {
			case <-ctx.Done():
				log.Println("received context done, stop sending jobs")
				return
			case <-t.stopAnnouncementCh:
				log.Println("received stop signal, stop sending jobs")
				return
			default:
				t.getAnnouncementCh <- jobs.FetchAnnouncementJob{Page: currentPage, PageSize: pageSize, Keyword: keyword, Emiten: emiten}
				currentPage++
			}
		}
	}(ctx, page)

	var mutex sync.Mutex
	responses := make([]response.Response, 0, 100)
loop:
	for {
		select {
		case <-ctx.Done():
			log.Println("received context done, stop listening for result")
			return ctx.Err()
		case <-t.stopAnnouncementCh:
			log.Println("received stop signal, stop listening for result")
			break loop
		case res, ok := <-t.resGetAnnouncementCh:
			if !ok {
				log.Println("channel is closed stop receiving from channel")
				break loop
			}
			if res.Err != nil {
				log.Println("failed to get announcement:", res.Err.Error())
				continue
			}
			if len(res.Replies) == 0 {
				log.Println("Replies is empty stopping")
				t.stopAnnouncementCh <- struct{}{}
				break loop
			}
			mutex.Lock()
			responses = append(responses, res.Response)
			mutex.Unlock()
			for _, reply := range res.Replies {
				for _, attachment := range reply.Attachments {
					log.Println("sending job to download attachment", attachment)
					t.downloadJobCh <- jobs.DownloadJob{Emiten: reply.Pengumuman.KodeEmiten, Attachment: attachment}
				}
			}
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Println("failed to get user home dir:", err.Error())
		return err
	}

	dir := path.Join(homeDir, "Downloads", "bloodhound")
	err = os.MkdirAll(dir, os.FileMode(0o755))
	if err != nil {
		log.Println("failed to create dir:", err.Error())
		return err
	}
	timestamp := time.Now().Format("2006-01-02_15:04:05")
	fileName := fmt.Sprintf("%s_responses.json", timestamp)
	filePath := filepath.Join(homeDir, "Downloads", "bloodhound", fileName)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(0o644))
	if err != nil {
		log.Println("failed to create file:", err.Error())
		return err
	}

	err = json.NewEncoder(file).Encode(responses)
	if err != nil {
		log.Println("failed to decode json to file:", err.Error())
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
