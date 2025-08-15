package common

import (
	"log"
	"os"
	"path"
	"path/filepath"
)

func CreateFile(dir, title string) (*os.File, error) {
	fp := filepath.Join(dir, title)
	file, err := os.OpenFile(fp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, os.FileMode(0o644))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func CreateDir(dirName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalln(err.Error())
	}
	dir := path.Join(home, "Downloads", "bloodhound", dirName)
	err = os.MkdirAll(dir, os.FileMode(0o755))
	if err != nil {
		log.Fatalln(err.Error())
	}
	return dir
}
