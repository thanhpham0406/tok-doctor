package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/thanhpham0406/tok-doctor/internal/cli"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))

	if err := cli.Execute(context.Background(), os.Stdout, os.Stderr, logger); err != nil {
		logger.Error("tok failed", "err", err)
		os.Exit(1)
	}
}
