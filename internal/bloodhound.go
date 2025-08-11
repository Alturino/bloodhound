package internal

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	req "github.com/imroc/req/v3"

	"github.com/Alturino/bloodhound/internal/response"
)

type Track struct {
	http *req.Client
}

func NewTrack(http *req.Client) *Track {
	return &Track{http: http}
}

func (t Track) Track(
	ctx context.Context,
	emiten, keyword string,
	page, pageSize int,
) response.Response {
	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"
	pageStr := strconv.Itoa(page)
	pageSizeStr := strconv.Itoa(pageSize)
	resp, err := t.http.R().
		SetHeader("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:141.0) Gecko/20100101 Firefox/141.0").
		SetHeader("Referer", "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/").
		SetHeader("Host", "idx.co.id").
		SetHeader("Connection", "keep-alive").
		SetHeader("Sec-Fetch-Dest", "empty").
		SetHeader("Sec-Fetch-Mode", "cors").
		SetHeader("Sec-Fetch-Site", "same-origin").
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

	// if err = json.NewEncoder(os.Stdout).Encode(res); err != nil {
	// 	log.Fatalf("Failed to encode response: %v", err)
	// }

	t.Download(ctx, res)
	return res
}

func (t Track) Download(ctx context.Context, data response.Response) {
	pool := 5
	downloadJob := make(chan DownloadJob, pool)
	downloadResult := make(chan DownloadRes, pool)

	for _, reply := range data.Replies {
		emiten := reply.Pengumuman.KodeEmiten
		log.Println("Downloading attachments for", emiten, reply.Pengumuman.JudulPengumuman)

		dir := createDir(emiten)
		for i := range pool {
			log.Println("Starting download worker", i)
			job := <-downloadJob
			log.Println(
				"Download worker",
				i,
				"Received job to download",
				job.File.Name(),
				"with url",
				job.URL,
			)
			go downloadFile(job.Ctx, job.URL, job.Request, job.File, downloadResult, i)
		}

		for i, attachment := range reply.Attachments {
			title := attachment.OriginalFilename
			file := createFile(dir, title)
			log.Println("Sending job to download", title, "with url", attachment.FullSavePath)
			downloadJob <- DownloadJob{Ctx: ctx, URL: attachment.FullSavePath, Request: t.http.R(), File: file, CurrentAttachment: i, TotalAttachment: len(reply.Attachments)}
			log.Println("Sent job to download", title, "with url", attachment.FullSavePath)
		}

		for res := range downloadResult {
			if res.Err != nil {
				log.Fatalln("Download failed", res.Err.Error())
			}
			log.Println("Download success", res.URL)
		}
	}
}

func createDir(emiten string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalln(err.Error())
	}
	dir := path.Join(home, "Downloads", "bloodhound", emiten)
	err = os.MkdirAll(dir, os.FileMode(0o755))
	if err != nil {
		log.Fatalln(err.Error())
	}
	return dir
}

func createFile(dir, title string) *os.File {
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "/", "_")
	title = strings.ToLower(title)
	filename := strings.Join(strings.Split(title, " "), "_")
	fp := filepath.Join(dir, filename)
	file, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY, os.FileMode(0o644))
	if err != nil {
		log.Fatalln(err.Error())
	}
	return file
}

func downloadFile(
	ctx context.Context,
	url string,
	request *req.Request,
	file *os.File,
	resCh chan<- DownloadRes,
	pool int,
) {
	log.Println("Download worker", pool, "Downloading file from", url)
	resp, err := request.SetHeader("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:141.0) Gecko/20100101 Firefox/141.0").
		SetHeader("Referer", "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/").
		SetHeader("Host", "idx.co.id").
		SetHeader("Connection", "keep-alive").
		SetHeader("Sec-Fetch-Dest", "empty").
		SetHeader("Sec-Fetch-Mode", "cors").
		SetHeader("Sec-Fetch-Site", "same-origin").
		SetContext(ctx).
		Get(url)
	if err != nil {
		resCh <- DownloadRes{Err: err}
		log.Println("Download worker", pool, "failed to download file from", url, ":", err.Error())
		return
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		resCh <- DownloadRes{Err: err}
		log.Println(
			"Download worker",
			pool,
			"failed to write file to",
			file.Name(),
			":",
			err.Error(),
		)
		return
	}
	defer file.Close()
	log.Println("Download worker", pool, "successfully downloaded file from", url)

	resCh <- DownloadRes{Err: nil, URL: url}
}

type DownloadJob struct {
	Ctx               context.Context
	URL               string
	Request           *req.Request
	File              *os.File
	TotalAttachment   int
	CurrentAttachment int
}

type DownloadRes struct {
	Err               error
	URL               string
	TotalAttachment   int
	CurrentAttachment int
}
