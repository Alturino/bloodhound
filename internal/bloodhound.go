package internal

import (
	"context"
	"encoding/json"
	"log"
	"os"

	req "github.com/imroc/req/v3"

	"github.com/Alturino/bloodhound/internal/response"
)

func Track(ctx context.Context) {
	url := "https://idx.co.id/primary/ListedCompany/GetAnnouncement"
	httpClient := req.ImpersonateFirefox()
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
		AddQueryParam("kodeEmiten", "AADI").
		AddQueryParam("indexFrom", "0").
		AddQueryParam("pageSize", "100").
		AddQueryParam("lang", "id").
		AddQueryParam("emitenType", "*").
		AddQueryParam("keyword", "").
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
}
