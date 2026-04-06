package storage

import (
	"context"
	"io"
)

// FakeStorage implements the storage.Storage interface for testing
type FakeStorage struct {
	Files map[string][]byte
}

func (f *FakeStorage) Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	if f.Files == nil {
		f.Files = make(map[string][]byte)
	}
	data, _ := io.ReadAll(r)
	f.Files[key] = data
	return nil
}

func (f *FakeStorage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	if f.Files == nil {
		return false, nil
	}
	data, ok := f.Files[key]
	if !ok {
		return false, nil
	}
	return len(data) > 0, nil
}
