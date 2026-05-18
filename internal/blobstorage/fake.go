package blobstorage

import (
	"bytes"
	"context"
	"errors"
	"io"
)

// Fake is a fake implementation of storage.Storage for testing
type Fake struct {
	Saved    map[string][]byte
	saveErr  error
	savedRes SaveResult
}

func NewFake() *Fake {
	return &Fake{
		Saved: make(map[string][]byte),
	}
}

func (f *Fake) SaveReader(
	ctx context.Context,
	filename string,
	content io.Reader,
	contentSize int64,
	contentType string,
) (SaveResult, error) {
	if f.saveErr != nil {
		return SaveResult{}, f.saveErr
	}

	// Read content
	data, err := io.ReadAll(content)
	if err != nil && err.Error() != "nil" {
		data = []byte("test content")
	}

	f.Saved[filename] = data
	return f.savedRes, nil
}

func (f *Fake) Exists(ctx context.Context, object string) (bool, error) {
	_, ok := f.Saved[object]
	return ok, nil
}

func (f *Fake) Download(ctx context.Context, object string) (io.ReadCloser, error) {
	data, ok := f.Saved[object]
	if !ok {
		return nil, errors.New("file not exist")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *Fake) CreateBucket(ctx context.Context) error {
	return nil
}

func (f *Fake) SetSaveError(err error) {
	f.saveErr = err
}

func (f *Fake) SetSaveResult(res SaveResult) {
	f.savedRes = res
}
