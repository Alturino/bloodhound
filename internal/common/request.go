package common

import "github.com/imroc/req/v3"

func BuildRequest(request *req.Request) *req.Request {
	return request.SetHeader("User-Agent", "Mozilla/5.0 (Linux; Android 10; SM-A205U) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Mobile Safari/537.36").
		SetHeader("Host", "idx.co.id").
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip").
		SetHeader("Referer", "https://www.idx.co.id/id/perusahaan-tercatat/keterbukaan-informasi/").
		SetHeader("sec-ch-ua-platform", `"Android"`)
}
