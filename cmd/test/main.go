// Package main implements the entry point for the oneapi-test command-line tool.
//
// Since end‑to‑end test suites are highly complex, I believe we should tackle them
// case by case—testing and fixing each one sequentially. After a test passes,
// we should add sufficient regression checks before proceeding to the next,
// so that ultimately all tests succeed.
//
// BTW, running all tests is costly, try to run specific tests during development.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	_ "github.com/joho/godotenv/autoload"
)

// main configures logging, listens for termination signals, and runs the regression harness.
func main() {
	logger, err := glog.NewConsoleWithName("oneapi-test", glog.LevelInfo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %+v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := "run"
	if len(os.Args) > 1 {
		command = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}

	var execErr error
	switch command {
	case "", "run":
		execErr = run(ctx, logger)
	case "generate":
		execErr = generate(ctx, logger)
	case "audio":
		execErr = audio(ctx, logger, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		os.Exit(1)
	}

	if execErr != nil {
		logger.Error("command failed", zap.String("command", command), zap.Error(execErr))
		os.Exit(1)
	}

	logger.Info("command completed", zap.String("command", command))
}
