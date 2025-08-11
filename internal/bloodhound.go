package internal

import (
	"context"
	"encoding/json"
	"fmt"
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

func Track(ctx context.Context, emiten, keyword string, page, pageSize int) response.Response {
	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"
	httpClient := req.ImpersonateFirefox()
	pageStr := strconv.Itoa(page)
	pageSizeStr := strconv.Itoa(pageSize)
	resp, err := httpClient.R().
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

	if err = json.NewEncoder(os.Stdout).Encode(res); err != nil {
		log.Fatalf("Failed to encode response: %v", err)
	}
	return res
}

func Download(ctx context.Context, data response.Response) {
	for _, reply := range data.Replies {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalln(err.Error())
		}
		dir := path.Join(home, "Downloads", reply.Pengumuman.KodeEmiten)
		err = os.MkdirAll(dir, os.FileMode(0o755))
		if err != nil {
			log.Fatalln(err.Error())
		}

		filename := fmt.Sprintf(
			"%v_%s_%s.pdf",
			reply.Pengumuman.TglPengumuman.Format("2006_01_02T15_04_05"),
			strings.ToLower(reply.Pengumuman.KodeEmiten),
			strings.ToLower(reply.Pengumuman.JudulPengumuman),
		)
		fp := filepath.Join(dir, filename)
		file, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY, os.FileMode(0o644))
		if err != nil {
			log.Fatalln(err.Error())
		}
		defer file.Close()

		resp, err := req.ImpersonateFirefox().R().
			SetHeader("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:141.0) Gecko/20100101 Firefox/141.0").
			SetHeader("Referer", "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/").
			SetHeader("Host", "idx.co.id").
			SetHeader("Connection", "keep-alive").
			SetHeader("Sec-Fetch-Dest", "empty").
			SetHeader("Sec-Fetch-Mode", "cors").
			SetHeader("Sec-Fetch-Site", "same-origin").
			SetContext(ctx).
			Get(reply.Attachments[0].FullSavePath)
		if err != nil {
			log.Fatalln(err.Error())
		}
		defer resp.Body.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			log.Fatalln(err.Error())
		}
	}
}
