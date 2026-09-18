package main

import (
	"context"
	"os"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/cli"
)

func main() {
	os.Exit(cli.Run(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		cli.DefaultDependencies(),
	))
}
