package common

import "github.com/imroc/req/v3"

func BuildRequest(request *req.Request) *req.Request {
	return request.SetHeader("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:141.0) Gecko/20100101 Firefox/141.0").
		SetHeader("Host", "idx.co.id").
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip")
}
