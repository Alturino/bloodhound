package blobstorage

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/alturino/bloodhound/config"
)

func newTestStorage(t *testing.T) (Storage, string) {
	dir := t.TempDir()
	cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
	logger := slog.Default()
	tracer := noop.Tracer{}
	return NewLocalFile(cfg, logger, tracer), dir
}

func TestLocalFile_NewLocalFile(t *testing.T) {
	tests := []struct {
		name    string
		config  config.Local
		wantNil bool
	}{
		{
			name: "enabled with valid dir",
			config: config.Local{
				Enabled:       true,
				BloodhoundDir: t.TempDir(),
			},
			wantNil: false,
		},
		{
			name: "enabled false returns noop",
			config: config.Local{
				Enabled: false,
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.Default()
			tracer := noop.Tracer{}
			storage := NewLocalFile(&tt.config, logger, tracer)
			isNoop := func() bool {
				_, ok := storage.(*noopLocalFile)
				return ok
			}()
			if tt.wantNil && !isNoop {
				t.Error("expected noopLocalFile, got different type")
			}
			if !tt.wantNil && isNoop {
				t.Error("expected localFile, got noop")
			}
		})
	}
}

func TestLocalFile_NewLocalFile_CreateBucketFails(t *testing.T) {
	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(parentDir, 0o555); err != nil {
		t.Fatalf("create readonly dir: %v", err)
	}
	readonlyPath := filepath.Join(parentDir, "nested")

	cfg := config.Local{
		Enabled:       true,
		BloodhoundDir: readonlyPath,
	}
	logger := slog.Default()
	tracer := noop.Tracer{}
	storage := NewLocalFile(&cfg, logger, tracer)

	_, ok := storage.(*noopLocalFile)
	if !ok {
		t.Error("expected noopLocalFile when CreateBucket fails")
	}
}

func TestLocalFile_SaveReader(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, Storage, string)
	}{
		{
			name: "write new file",
			run: func(t *testing.T, s Storage, filename string) {
				content := bytes.NewReader([]byte("test content"))
				result, err := s.SaveReader(
					context.Background(),
					filename,
					content,
					0,
					"application/octet-stream",
				)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result.UploadInfo.ChecksumSHA256 == "" {
					t.Error("expected checksum, got empty")
				}
			},
		},
		{
			name: "write in nested directory",
			run: func(t *testing.T, s Storage, _ string) {
				content := bytes.NewReader([]byte("nested content"))
				_, err := s.SaveReader(
					context.Background(),
					"subdir/anotherdir/file.txt",
					content,
					0,
					"text/plain",
				)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "overwrite existing file",
			run: func(t *testing.T, s Storage, filename string) {
				content := bytes.NewReader([]byte("first write"))
				_, err := s.SaveReader(context.Background(), filename, content, 0, "text/plain")
				if err != nil {
					t.Errorf("first write error: %v", err)
				}
				content = bytes.NewReader([]byte("second write"))
				_, err = s.SaveReader(context.Background(), filename, content, 0, "text/plain")
				if err != nil {
					t.Errorf("overwrite error: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestStorage(t)
			tt.run(t, s, "testfile.txt")
		})
	}
}

func TestLocalFile_SaveReader_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	readonlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readonlyDir, 0o555); err != nil {
		t.Fatalf("create readonly dir: %v", err)
	}

	cfg := &config.Local{Enabled: true, BloodhoundDir: readonlyDir}
	logger := slog.Default()
	tracer := noop.Tracer{}
	s := NewLocalFile(cfg, logger, tracer)

	content := bytes.NewReader([]byte("test"))
	_, err := s.SaveReader(context.Background(), "newfile.txt", content, 0, "text/plain")
	if err == nil {
		t.Error("expected error when writing to readonly dir")
	}
}

func TestLocalFile_Exists(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) (Storage, string, func())
		check func(*testing.T, bool, error)
	}{
		{
			name: "file exists with size > 0",
			setup: func(t *testing.T) (Storage, string, func()) {
				dir := t.TempDir()
				testFile := filepath.Join(dir, "exists.txt")
				if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
					t.Fatalf("write test file: %v", err)
				}
				cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
				logger := slog.Default()
				tracer := noop.Tracer{}
				return NewLocalFile(cfg, logger, tracer), "exists.txt", func() {}
			},
			check: func(t *testing.T, exists bool, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !exists {
					t.Error("expected file to exist")
				}
			},
		},
		{
			name: "file does not exist",
			setup: func(t *testing.T) (Storage, string, func()) {
				s, _ := newTestStorage(t)
				return s, "nonexistent.txt", func() {}
			},
			check: func(t *testing.T, exists bool, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if exists {
					t.Error("expected file to not exist")
				}
			},
		},
		{
			name: "file exists but empty",
			setup: func(t *testing.T) (Storage, string, func()) {
				dir := t.TempDir()
				testFile := filepath.Join(dir, "empty.txt")
				if err := os.WriteFile(testFile, []byte(""), 0o644); err != nil {
					t.Fatalf("write empty file: %v", err)
				}
				cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
				logger := slog.Default()
				tracer := noop.Tracer{}
				return NewLocalFile(cfg, logger, tracer), "empty.txt", func() {}
			},
			check: func(t *testing.T, exists bool, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if exists {
					t.Error("expected empty file to not exist (size 0)")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, filename, cleanup := tt.setup(t)
			defer cleanup()
			exists, err := s.Exists(context.Background(), filename)
			tt.check(t, exists, err)
		})
	}
}

func TestLocalFile_Exists_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skip permission test when running as root")
	}
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	testFile := filepath.Join(subdir, "noperm.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Chmod(subdir, 0o000); err != nil {
		t.Fatalf("chmod subdir: %v", err)
	}

	cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
	logger := slog.Default()
	tracer := noop.Tracer{}
	s := NewLocalFile(cfg, logger, tracer)

	_, err := s.Exists(context.Background(), filepath.Join("subdir", "noperm.txt"))
	if err == nil {
		t.Error("expected error for permission denied")
	}

	os.Chmod(subdir, 0o755)
}

func TestLocalFile_Download(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) (Storage, string, func())
		check func(*testing.T, io.ReadCloser, error)
	}{
		{
			name: "read file content",
			setup: func(t *testing.T) (Storage, string, func()) {
				dir := t.TempDir()
				testFile := filepath.Join(dir, "readable.txt")
				expected := []byte("readable content")
				if err := os.WriteFile(testFile, expected, 0o644); err != nil {
					t.Fatalf("write test file: %v", err)
				}
				cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
				logger := slog.Default()
				tracer := noop.Tracer{}
				return NewLocalFile(cfg, logger, tracer), "readable.txt", func() {}
			},
			check: func(t *testing.T, rc io.ReadCloser, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if rc == nil {
					t.Fatal("expected reader, got nil")
				}
				data, _ := io.ReadAll(rc)
				rc.Close()
				if !bytes.Equal(data, []byte("readable content")) {
					t.Errorf("expected content, got %s", string(data))
				}
			},
		},
		{
			name: "read file with binary content",
			setup: func(t *testing.T) (Storage, string, func()) {
				dir := t.TempDir()
				testFile := filepath.Join(dir, "binary.bin")
				binary := make([]byte, 256)
				for i := range binary {
					binary[i] = byte(i % 256)
				}
				if err := os.WriteFile(testFile, binary, 0o644); err != nil {
					t.Fatalf("write binary file: %v", err)
				}
				cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
				logger := slog.Default()
				tracer := noop.Tracer{}
				return NewLocalFile(cfg, logger, tracer), "binary.bin", func() {}
			},
			check: func(t *testing.T, rc io.ReadCloser, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				data, _ := io.ReadAll(rc)
				rc.Close()
				if len(data) != 256 {
					t.Errorf("expected 256 bytes, got %d", len(data))
				}
			},
		},
		{
			name: "read empty file",
			setup: func(t *testing.T) (Storage, string, func()) {
				dir := t.TempDir()
				testFile := filepath.Join(dir, "empty.bin")
				if err := os.WriteFile(testFile, []byte{}, 0o644); err != nil {
					t.Fatalf("write empty file: %v", err)
				}
				cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
				logger := slog.Default()
				tracer := noop.Tracer{}
				return NewLocalFile(cfg, logger, tracer), "empty.bin", func() {}
			},
			check: func(t *testing.T, rc io.ReadCloser, err error) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				data, _ := io.ReadAll(rc)
				rc.Close()
				if len(data) != 0 {
					t.Errorf("expected 0 bytes, got %d", len(data))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, filename, cleanup := tt.setup(t)
			defer cleanup()
			rc, err := s.Download(context.Background(), filename)
			tt.check(t, rc, err)
		})
	}
}

func TestLocalFile_Download_Errors(t *testing.T) {
	s, _ := newTestStorage(t)

	_, err := s.Download(context.Background(), "nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLocalFile_Download_PermissionDenied(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "noperm.bin")
	if err := os.WriteFile(testFile, []byte("secret"), 0o000); err != nil {
		t.Fatalf("write file with no perms: %v", err)
	}

	cfg := &config.Local{Enabled: true, BloodhoundDir: dir}
	logger := slog.Default()
	tracer := noop.Tracer{}
	s := NewLocalFile(cfg, logger, tracer)

	_, err := s.Download(context.Background(), "noperm.bin")
	if err == nil {
		t.Error("expected error for permission denied")
	}
}

func TestLocalFile_CreateBucket(t *testing.T) {
	t.Run("create new directory", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.Local{Enabled: true, BloodhoundDir: filepath.Join(dir, "newbucket")}
		logger := slog.Default()
		tracer := noop.Tracer{}
		s := NewLocalFile(cfg, logger, tracer)
		err := s.CreateBucket(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if _, statErr := os.Stat(cfg.BloodhoundDir); os.IsNotExist(statErr) {
			t.Error("directory not created")
		}
	})

	t.Run("idempotent - directory already exists", func(t *testing.T) {
		s, _ := newTestStorage(t)
		err := s.CreateBucket(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestLocalFile_PathTraversal(t *testing.T) {
	s, dir := newTestStorage(t)

	testCases := []struct {
		name     string
		filename string
	}{
		{"relative parent traversal", "../../../etc/passwd"},
		{"nested parent traversal", "subdir/../../../etc/passwd"},
		{"absolute path", "/etc/passwd"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			content := bytes.NewReader([]byte("malicious"))
			_, err := s.SaveReader(context.Background(), tc.filename, content, 0, "text/plain")
			if err != nil {
				return
			}
			savedPath := filepath.Join(dir, filepath.Base(tc.filename))
			if _, statErr := os.Stat(savedPath); statErr == nil {
				t.Errorf("file saved outside BloodhoundDir: %s", tc.filename)
			}
		})
	}
}

func TestLocalFile_Concurrent(t *testing.T) {
	s, _ := newTestStorage(t)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			content := bytes.NewReader([]byte("concurrent content"))
			s.SaveReader(context.Background(), "concurrent.txt", content, 0, "text/plain")
		}()
	}
	wg.Wait()

	exists, err := s.Exists(context.Background(), "concurrent.txt")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected file to exist after concurrent writes")
	}
}
