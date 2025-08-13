package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	req "github.com/imroc/req/v3"

	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/response"
)

type Track struct {
	http *req.Client
}

func NewTrack(http *req.Client) *Track {
	return &Track{http: http}
}

// TODO: TrackTillEmpty should be checking the latest document from the idx.co.id between the latest file in database or json file before get the whole file
func (t Track) TrackTillEmpty(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) {
	dir := createDir("responses")
	timestamp := time.Now().Format("2006-01-02_15:04:05")
	filename := filepath.Join(dir, fmt.Sprintf("%s_response.json", timestamp))

	file, err := createFile(dir, filename)
	if err != nil {
		log.Fatalln("Failed to create file:", err.Error())
	}
	defer file.Close()

	responses := make([]response.Response, 0, 100)
	for {
		response := t.Track(ctx, emiten, keyword, page, pageSize)
		responses = append(responses, response)
		if len(response.Replies) == 0 {
			break
		}
		page++
	}

	if err := json.NewEncoder(file).Encode(responses); err != nil {
		log.Fatalln(err.Error())
	}
}

func (t Track) Track(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) response.Response {
	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"
	pageStr := strconv.Itoa(page)
	pageSizeStr := strconv.Itoa(pageSize)
	resp, err := buildRequest(t.http.R()).
		SetHeader("Sec-Fetch-Dest", "document").
		SetHeader("Sec-Fetch-Mode", "navigate").
		SetHeader("Sec-Fetch-Site", "cross-site").
		// EnableDump().
		// EnableDumpTo(os.Stdout).
		SetContext(ctx).
		AddQueryParam("kodeEmiten", emiten).
		AddQueryParam("indexFrom", pageStr).
		AddQueryParam("pageSize", pageSizeStr).
		AddQueryParam("lang", "id").
		AddQueryParam("emitenType", "*").
		AddQueryParam("keyword", keyword).
		Get(url)
	if err != nil {
		log.Fatalf("Failed to get response: %v", err)
	}

	var res response.Response
	if err = json.NewDecoder(resp.Body).Decode(&res); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}

	t.Download(ctx, res)
	return res
}

func (t Track) Download(ctx context.Context, data response.Response) {
	pool := 10
	stopCh := make(chan struct{}, 1)
	defer close(stopCh)
	jobCh := make(chan jobs.DownloadJob, pool)
	defer close(jobCh)
	resCh := make(chan jobs.DownloadRes, pool)
	defer close(resCh)
	go t.workerPool(ctx, pool, stopCh, jobCh, resCh)
	for i, reply := range data.Replies {
		emiten := reply.Pengumuman.KodeEmiten
		log.Println("Downloading attachments for", emiten, reply.Pengumuman.JudulPengumuman)
		for j, attachment := range reply.Attachments {
			title := attachment.OriginalFilename
			title = strings.TrimSpace(title)
			title = strings.ReplaceAll(title, "/", "_")
			title = strings.ToLower(title)
			title = strings.Join(strings.Split(title, " "), "_")
			jobCh <- jobs.DownloadJob{URL: attachment.FullSavePath, Filename: title, Emiten: emiten, ReplyID: i, AttachmentID: j}
		}

		for j, attachment := range reply.Attachments {
			log.Println(
				"Waiting for worker to finish downloading",
				"URL", attachment.FullSavePath,
				"attachment", j,
			)
			res := <-resCh
			log.Println(
				"Worker", res.WorkerID, " finished",
				"URL", res.URL,
				"ReplyID", res.ReplyID,
				"attachment", res.AttachmentID,
			)
			if res.Err != nil {
				log.Println(
					"Worker", res.WorkerID, "failed to download",
					"URL", res.URL,
					"ReplyID", res.ReplyID,
					"attachment", res.AttachmentID,
					":", res.Err.Error(),
				)
				continue
			}
			log.Println(
				"Worker", res.WorkerID, "successfully to download",
				"URL", res.URL,
				"ReplyID", res.ReplyID,
				"attachment", res.AttachmentID,
			)
		}
	}
}

func createDir(dirName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalln(err.Error())
	}
	dir := path.Join(home, "Downloads", "bloodhound", dirName)
	err = os.MkdirAll(dir, os.FileMode(0o755))
	if err != nil {
		log.Fatalln(err.Error())
	}
	return dir
}

func createFile(dir, title string) (*os.File, error) {
	fp := filepath.Join(dir, title)
	file, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(0o644))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (t Track) downloadFile(
	ctx context.Context,
	url string,
	file *os.File,
) error {
	log.Println("Download worker", "Downloading file from", url)
	resp, err := buildRequest(t.http.R()).
		SetHeader("Sec-Fetch-Dest", "empty").
		SetHeader("Sec-Fetch-Mode", "cors").
		SetHeader("Sec-Fetch-Site", "same-origin").
		SetContext(ctx).
		Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	defer file.Close()

	log.Println("Write downloaded file to", file.Name())
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func buildRequest(request *req.Request) *req.Request {
	return request.SetHeader("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:141.0) Gecko/20100101 Firefox/141.0").
		SetHeader("Host", "idx.co.id").
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip")
}

// blocking worker pool need to be called with goroutine
func (t Track) workerPool(
	ctx context.Context,
	pool int,
	stopCh <-chan struct{},
	jobCh <-chan jobs.DownloadJob,
	resCh chan<- jobs.DownloadRes,
) {
	select {
	case <-stopCh:
		return
	case <-ctx.Done():
		return
	default:
		for i := range pool {
			log.Println("started worker", i)
			go func(workerID int) {
				for job := range jobCh {
					dir := createDir(job.Emiten)
					file, err := createFile(dir, job.Filename)
					if err != nil {
						if os.IsExist(err) {
							log.Println("File already exists")
							path := filepath.Join(dir, job.Filename)
							log.Println("checking existing file at", path)
							info, err := os.Stat(path)
							if err != nil {
								log.Println("Failed to get info of the existing file:", err)
								resCh <- jobs.DownloadRes{Err: errors.New("failed to get info of the existing file"), URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
								continue
							}
							log.Println("Existing file size:", info.Size())
							if info.Size() > 0 {
								log.Println("File is not empty, skipping download")
								resCh <- jobs.DownloadRes{Err: errors.New("file is not empty skipping download"), URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
								continue
							}
							log.Println("Existing file was empty, downloading again")
							file, err = os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o644)
							if err != nil {
								log.Println("Failed to open empty file for writing:", err)
								resCh <- jobs.DownloadRes{
									Err:          err,
									URL:          job.URL,
									AttachmentID: job.AttachmentID,
									ReplyID:      job.ReplyID,
									WorkerID:     workerID,
								}
								continue
							}
							log.Println("File was empty, continuing with download.")
						} else {
							log.Println("Failed to create file:", err.Error())
							resCh <- jobs.DownloadRes{Err: err, URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
							continue
						}
					}
					log.Println(
						"worker", workerID,
						"downloading", job.URL,
						"to", job.Filename,
						"in directory", dir,
					)
					err = t.downloadFile(ctx, job.URL, file)
					if err != nil {
						log.Println("worker", workerID, "failed to download", job.URL)
						resCh <- jobs.DownloadRes{Err: err, URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
						continue
					}
					log.Println("worker", workerID, "downloaded", job.URL)
					resCh <- jobs.DownloadRes{Err: nil, URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
				}
			}(i)
		}
	}
}
