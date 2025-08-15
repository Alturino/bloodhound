package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	req "github.com/imroc/req/v3"

	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/jobs"
	"github.com/Alturino/bloodhound/internal/response"
	"github.com/Alturino/bloodhound/internal/worker"
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
	dir := common.CreateDir("responses")
	timestamp := time.Now().Format("2006-01-02_15:04:05")
	filename := fmt.Sprintf("%s_response.json", timestamp)

	file, err := common.CreateFile(dir, filename)
	if err != nil {
		log.Fatalln("Failed to create file:", err.Error())
	}
	defer file.Close()

	responses := make([]response.Response, 0, 100)
	currentPage := page
	for {
		res := t.Track(ctx, "", "", currentPage, 100)
		if len(res.Replies) == 0 {
			break
		}
		responses = append(responses, res)
		currentPage++
		time.Sleep(time.Second * 5)
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
	resp, err := common.BuildRequest(t.http.R()).
		SetHeader("Sec-Fetch-Dest", "document").
		SetHeader("Sec-Fetch-Mode", "navigate").
		SetHeader("Sec-Fetch-Site", "cross-site").
		EnableDump().
		EnableDumpTo(os.Stdout).
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

	// t.download(ctx, res)
	return res
}

func (t Track) download(ctx context.Context, data response.Response) {
	pool := 10

	jobCh := make(chan jobs.DownloadJob, pool)
	defer close(jobCh)

	resCh := make(chan jobs.DownloadRes, pool)
	defer close(resCh)

	go worker.WorkerPool(ctx, pool, jobCh, resCh, t.downloadWorker)

	for i, reply := range data.Replies {
		emiten := reply.Pengumuman.KodeEmiten
		log.Println("Downloading attachments for", emiten, reply.Pengumuman.JudulPengumuman)

		for j, attachment := range reply.Attachments {
			go func() {
				title := attachment.OriginalFilename
				title = strings.TrimSpace(title)
				title = strings.ReplaceAll(title, "/", "_")
				title = strings.ToLower(title)
				title = strings.Join(strings.Split(title, " "), "_")
				jobCh <- jobs.DownloadJob{URL: attachment.FullSavePath, Filename: title, Emiten: emiten, ReplyID: i, AttachmentID: j}
			}()
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

func (t Track) downloadWorker(
	ctx context.Context,
	workerID int,
	jobCh <-chan jobs.DownloadJob,
	resCh chan<- jobs.DownloadRes,
) {
	for job := range jobCh {
		dir := common.CreateDir(job.Emiten)
		file, err := common.CreateFile(dir, job.Filename)
		if err != nil {
			log.Println("Failed to create file:", err.Error())
			resCh <- jobs.DownloadRes{Err: err, URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
			continue
		}
		log.Println(
			"worker", workerID,
			"downloading", job.URL,
			"to", job.Filename,
			"in directory", dir,
		)
		err = common.DownloadFile(ctx, t.http.R(), job.URL, file)
		if err != nil {
			log.Println("worker", workerID, "failed to download", job.URL)
			resCh <- jobs.DownloadRes{Err: err, URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
			continue
		}
		log.Println("worker", workerID, "downloaded", job.URL)
		resCh <- jobs.DownloadRes{Err: nil, URL: job.URL, AttachmentID: job.AttachmentID, ReplyID: job.ReplyID, WorkerID: workerID}
	}
}
