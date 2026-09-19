package blobstorage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/alturino/bloodhound/internal/config"
	"github.com/alturino/bloodhound/internal/constants"
	"github.com/alturino/bloodhound/internal/telemetry"
)

type localFile struct {
	config *config.Local
	logger *slog.Logger
	tracer trace.Tracer
}

func NewLocalFile(config *config.Local, logger *slog.Logger, tracer trace.Tracer) Storage {
	if !config.Enabled {
		return &noopLocalFile{}
	}
	fs := &localFile{config: config, logger: logger, tracer: tracer}
	if err := fs.CreateBucket(context.Background()); err != nil {
		logger.Warn("create bucket", slog.Any("error", err))
	}
	return fs
}

// Upload uploads a file to the storage
func (f *localFile) Upload(
	ctx context.Context,
	filename string,
	content io.Reader,
	contentSize int64,
	contentType string,
) (SaveResult, error) {
	ctx, span := f.tracer.Start(
		ctx,
		"blobstorage.localFile.Upload",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := f.logger.With(slog.String("tag", "blobstorage.localFile.Upload"))

	dir := f.config.BloodhoundDir

	logger.DebugContext(ctx, "creating dir")
	span.AddEvent("creating dir")
	dir = filepath.Join(dir, filepath.Dir(filename))
	if err := os.MkdirAll(dir, os.FileMode(0o755)); err != nil {
		err = fmt.Errorf("creating dir: %w", err)
		telemetry.RecordError(span, err)
		return SaveResult{}, err
	}
	logger.DebugContext(ctx, "created dir")
	span.AddEvent("created dir")

	logger.DebugContext(ctx, "creating file")
	span.AddEvent("creating file")
	fp := filepath.Join(dir, filepath.Base(filename))
	file, err := os.Create(fp)
	if err != nil {
		err = fmt.Errorf("creating file: %w", err)
		telemetry.RecordError(span, err)
		return SaveResult{}, err
	}
	logger.DebugContext(ctx, "created file")
	span.AddEvent("created file")

	span.AddEvent("saving file locally")
	logger.DebugContext(ctx, "saving file locally")
	hash := sha256.New()
	tee := io.TeeReader(content, hash)
	if _, err := io.Copy(file, tee); err != nil {
		err = fmt.Errorf("save reader: %w", err)
		telemetry.RecordError(span, err)
		return SaveResult{}, err
	}
	saveres := SaveResult{
		UploadInfo: UploadInfo{
			Bucket:         f.config.BloodhoundDir,
			Key:            fp,
			Location:       fp,
			ChecksumSHA256: base64.StdEncoding.EncodeToString(hash.Sum(nil)),
		},
	}
	logger = logger.With(slog.Any(constants.LocalSaveResult, saveres))
	logger.DebugContext(ctx, "saved file locally")
	span.AddEvent("saved file locally")

	return saveres, nil
}

// Exists checks if a file exists in the storage
func (f *localFile) Exists(ctx context.Context, relPath string) (bool, error) {
	_, span := f.tracer.Start(
		ctx,
		"blobstorage.localFile.Exists",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	logger := f.logger.With(slog.String("tag", "blobstorage.localFile.Exists"))

	fp := filepath.Join(f.config.BloodhoundDir, relPath)

	logger.DebugContext(ctx, "checking file")
	span.AddEvent("checking file")
	fi, err := os.Stat(fp)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		err = fmt.Errorf("checking file: %v", err)
		return false, err
	}
	if fi.Size() < 0 {
		return false, nil
	}
	logger.DebugContext(ctx, "checked file", slog.Bool("is_exists", true))
	span.AddEvent("checking file")

	return true, nil
}

// Download downloads a file from the storage
func (f *localFile) Download(ctx context.Context, object string) (io.ReadCloser, error) {
	fp := filepath.Join(f.config.BloodhoundDir, object)

	_, span := f.tracer.Start(
		ctx,
		"blobstorage.localFile.Download",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(constants.Filepath, fp)),
	)
	defer span.End()

	logger := f.logger.With(
		slog.String("tag", "blobstorage.localFile.Download"),
		slog.String(constants.Filepath, fp),
	)

	logger.DebugContext(ctx, "opening file")
	span.AddEvent("opening file")
	file, err := os.Open(fp)
	if err != nil {
		err = fmt.Errorf("opening file: %v", err)
		return nil, err
	}
	logger.DebugContext(ctx, "opened file")
	span.AddEvent("opened file")

	return file, err
}

// CreateBucket creates a bucket if it doesn't exist
func (f *localFile) CreateBucket(ctx context.Context) error {
	fp := filepath.Clean(f.config.BloodhoundDir)
	_, span := f.tracer.Start(
		ctx,
		"blobstorage.localFile.CreateBucket",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(constants.Filepath, fp)),
	)
	defer span.End()

	logger := f.logger.With(
		slog.String("tag", "blobstorage.localFile.CreateBucket"),
		slog.String(constants.Filepath, fp),
	)

	logger.DebugContext(ctx, "creating dir")
	span.AddEvent("creating dir")
	if err := os.MkdirAll(fp, os.FileMode(0o755)); err != nil {
		err = fmt.Errorf("creating dir: %v", err)
		return err
	}
	logger.DebugContext(ctx, "created dir")
	span.AddEvent("created dir")

	return nil
}
