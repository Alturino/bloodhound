package nfs

import (
	"context"
	"fmt"
	"sync"

	"github.com/minio/minio-go/v7"

	"github.com/Alturino/bloodhound/internal/config"
)

func Get(ctx context.Context, config config.Config) (*minio.Client, error) {
	endpoint := fmt.Sprintf("%s:%d", config.Minio.Host, config.Minio.Port)
	return sync.OnceValues(func() (*minio.Client, error) {
		client, err := minio.New(endpoint, &minio.Options{})
		if err != nil {
			err = fmt.Errorf("error creating minio client: %w", err)
			return nil, err
		}
		if err := client.MakeBucket(ctx, "bloodhound", minio.MakeBucketOptions{}); err != nil {
			err = fmt.Errorf("error creating bucket: %w", err)
			return nil, err
		}
		return client, nil
	})()
}
