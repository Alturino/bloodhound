package common

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/Alturino/bloodhound/internal/logging"
)

var once sync.Once

var (
	HomeDir       string
	BloodhoundDir string
)

func GetHomeDir() string {
	logger := logging.Get()
	once.Do(func() {
		dir, err := os.UserHomeDir()
		if err != nil {
			logger.Fatal().Err(err).Msg(err.Error())
		}
		HomeDir = dir
		BloodhoundDir = filepath.Join(dir, "Downloads", "bloodhound")
	})
	return HomeDir
}
