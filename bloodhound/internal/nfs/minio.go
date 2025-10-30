package nfs

import (
	"context"
	"fmt"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"

	"github.com/Alturino/bloodhound/internal/config"
)

func Get(ctx context.Context, config config.Config) (*minio.Client, error) {
	endpoint := fmt.Sprintf("%s:%d", config.Minio.Host, config.Minio.Port)
	return sync.OnceValues(func() (*minio.Client, error) {
		client, err := minio.New(endpoint, &minio.Options{
			Trace: otelhttptrace.NewClientTrace(ctx, otelhttptrace.WithInsecureHeaders()),
			Creds: credentials.NewStaticV4("minio", "minio_minio", ""),
		})
		if err != nil {
			err = fmt.Errorf("error creating minio client: %w", err)
			return nil, err
		}
		return client, nil
	})()
}
