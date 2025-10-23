package client

import (
	"context"

	"github.com/imroc/req/v3"

	"github.com/Alturino/bloodhound/internal/common"
	"github.com/Alturino/bloodhound/internal/middleware"
)

type HTTPClient struct {
	*req.Client
}

func NewHTTPClient(ctx context.Context) HTTPClient {
	return HTTPClient{
		req.ImpersonateChrome().
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
			SetOutputDirectory(common.BloodhoundDir).
			SetUserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36").
			DisableAutoReadResponse(),
	}
}
