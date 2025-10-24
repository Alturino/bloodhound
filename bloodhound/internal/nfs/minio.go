package nfs

import (
	"context"
	"fmt"
	"sync"

	"github.com/minio/minio-go/v7"

	"github.com/Alturino/bloodhound/internal/config"
)

var (
	once   sync.Once
	client *minio.Client
)

func Get(ctx context.Context, config config.Config) (*minio.Client, error) {
	endpoint := fmt.Sprintf("%s:%d", config.Minio.Host, config.Minio.Port)
	var err error
	once.Do(func() {
		client, err = minio.New(endpoint, &minio.Options{})
		if err != nil {
			err = fmt.Errorf("error creating minio client: %w", err)
			return
		}
		client.MakeBucket(ctx, "bloodhound", minio.MakeBucketOptions{})
	})
	return client, err
}
