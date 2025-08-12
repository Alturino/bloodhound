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
	"sync"

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

		// SetHeader("Referer", "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/").
		SetHeader("Host", "idx.co.id").
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip").
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

	// if err = json.NewEncoder(os.Stdout).Encode(res); err != nil {
	// 	log.Fatalf("Failed to encode response: %v", err)
	// }

	t.Download(ctx, res)
	return res
}

func (t Track) Download(ctx context.Context, data response.Response) {
	var replyWg sync.WaitGroup
	for _, reply := range data.Replies {
		replyWg.Add(1)
		go func() {
			defer replyWg.Done()
			emiten := reply.Pengumuman.KodeEmiten
			log.Println("Downloading attachments for", emiten, reply.Pengumuman.JudulPengumuman)
			dir := createDir(emiten)
			var attachmentWg sync.WaitGroup
			for _, attachment := range reply.Attachments {
				attachmentWg.Add(1)
				go func() {
					defer attachmentWg.Done()
					title := attachment.OriginalFilename
					file := createFile(dir, title)
					if err := downloadWorker(ctx, attachment.FullSavePath, t.http.R(), file); err != nil {
						log.Fatalln("Download failed", err.Error())
					}
				}()
			}
			attachmentWg.Wait()
		}()
	}
	replyWg.Wait()
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

func downloadWorker(
	ctx context.Context,
	url string,
	request *req.Request,
	file *os.File,
) error {
	log.Println("Download worker", "Downloading file from", url)
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
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	defer file.Close()

	return nil
}

type DownloadJob struct {
	URL               string
	Request           *req.Request
	File              *os.File
	TotalAttachment   int
	CurrentAttachment int
}

type DownloadRes struct {
	Err error
	URL string
}
