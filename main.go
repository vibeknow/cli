package main

import (
	"os"

	"github.com/vibeknow/cli/cmd"
	"github.com/vibeknow/cli/internal/clerr"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(clerr.ExitCodeFor(err))
	}
}
