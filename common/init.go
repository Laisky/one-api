package common

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

var (
	// Port holds the CLI flag indicating which port the HTTP server listens on.
	Port = flag.Int("port", 3000, "the listening port")

	// PrintVersion toggles a CLI mode that prints the binary version and exits.
	PrintVersion = flag.Bool("version", false, "print version and exit")

	// PrintHelp toggles a CLI mode that prints usage information and exits.
	PrintHelp = flag.Bool("help", false, "print help and exit")

	// LogDir captures the CLI flag that points to the directory storing log files.
	LogDir = flag.String("log-dir", "./logs", "specify the log directory")
)

// func printHelp() {
// 	fmt.Println("One API " + Version + " - All in one API service for OpenAI API.")
// 	fmt.Println("Copyright (C) 2026 Laisky. All rights reserved.")
// 	fmt.Println("GitHub: https://github.com/Laisky/one-api")
// 	fmt.Println("Usage: one-api [--port <port>] [--log-dir <log directory>] [--version] [--help]")
// }

// Init parses CLI flags, normalizes configuration defaults, and prepares logging destinations.
func Init() {
	flag.Parse()

	// if *PrintVersion {
	// 	fmt.Println(Version)
	// 	os.Exit(0)
	// }

	// if *PrintHelp {
	// 	printHelp()
	// 	os.Exit(0)
	// }

	if config.SessionSecretEnvValue != "" {
		if config.SessionSecretEnvValue == "random_string" {
			logger.Logger.Error("SESSION_SECRET is set to an example value, please change it to a random string.")
		} else {
			config.SessionSecret = config.SessionSecretEnvValue
		}
	}
	SQLitePath = config.SQLitePath
	if *LogDir != "" {
		expanded := expandLogDirPath(*LogDir)
		lg := logger.Logger.With(zap.String("log_dir", expanded))
		lg.Debug("starting to set log dir")

		var err error
		expanded, err = filepath.Abs(expanded)
		if err != nil {
			lg.Fatal("failed to get absolute log dir", zap.Error(err))
		}

		if err = os.MkdirAll(expanded, 0o777); err != nil {
			lg.Fatal("failed to create log dir", zap.Error(err))
		}

		lg.Info("set log dir", zap.String("log_dir", expanded))
		logger.LogDir = expanded
		*LogDir = expanded
	}
}
