package main

import (
	"context"
	"os"
	"time"

	"github.com/bocacorazon/hermoso/internal/app"
)

var version = "dev"

func main() {
	os.Exit(app.Run(context.Background(), os.Args[1:], app.Dependencies{
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		FS:      app.OSFilesystem{},
		Version: version,
		Now:     time.Now,
	}))
}
