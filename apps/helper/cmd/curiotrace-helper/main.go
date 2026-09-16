package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Nasu726/CurioTrace/apps/helper/internal/app"
	"github.com/Nasu726/CurioTrace/apps/helper/internal/protocol"
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		// Never print request payloads. Keep diagnostics to error classes only.
		fmt.Fprintln(os.Stderr, "curiotrace-helper:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	handler := app.NewHandler()
	for {
		message, err := protocol.Read(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		response := handler.Handle(message)
		if err := protocol.Write(out, response); err != nil {
			return err
		}
	}
}
