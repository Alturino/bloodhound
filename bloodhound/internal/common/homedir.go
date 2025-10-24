package common

import (
	"os"
	"path"
	"sync"

	"github.com/Alturino/bloodhound/internal/logging"
)

var once sync.Once

var (
	HomeDir       string
	BloodhoundDir string
	TempDir       string
)

func InitDir() string {
	logger := logging.Get()
	once.Do(func() {
		dir, err := os.UserHomeDir()
		if err != nil {
			logger.Fatal().Err(err).Msg(err.Error())
		}
		HomeDir = dir
		BloodhoundDir = path.Join(dir, "Downloads", "bloodhound")

		tempDir, err := os.MkdirTemp(path.Join(os.TempDir(), "bloodhound"), "bloodhound-*")
		if err != nil {
			logger.Fatal().Err(err).Msg(err.Error())
		}
		TempDir = tempDir
	})
	return HomeDir
}
