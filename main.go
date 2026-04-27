package main

import (
	"os"

	"github.com/alturino/bloodhound/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
