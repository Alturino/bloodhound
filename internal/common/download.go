package common

import (
	"context"
	"io"
	"log"
	"os"
	"time"

	req "github.com/imroc/req/v3"
)

func DownloadFile(
	ctx context.Context,
	request *req.Request,
	url string,
	file *os.File,
) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := BuildRequest(request).
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
