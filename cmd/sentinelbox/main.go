package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/TBG-Chance/SentinelBox/internal/app"
	"github.com/TBG-Chance/SentinelBox/internal/buildinfo"
	"github.com/TBG-Chance/SentinelBox/internal/config"
	"github.com/TBG-Chance/SentinelBox/internal/logging"
)

func main() {
	os.Exit(run())
}

func run() int {
	defaultConfig := strings.TrimSpace(os.Getenv("SENTINELBOX_CONFIG"))
	if defaultConfig == "" {
		defaultConfig = "/etc/sentinelbox/sentinelbox.toml"
	}
	configPath := flag.String("config", defaultConfig, "path to the SentinelBox TOML configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	build := buildinfo.Current()
	if *showVersion {
		fmt.Printf("sentinelbox %s revision=%s built=%s\n", build.Version, build.Revision, build.BuildTime)
		return 0
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SentinelBox configuration error: %v\n", err)
		return 1
	}
	logger, err := logging.New(cfg.Logging, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SentinelBox logging error: %v\n", err)
		return 1
	}
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	application, err := app.New(ctx, cfg, logger, build)
	if err != nil {
		logger.Error("SentinelBox startup failed", "error", err)
		return 1
	}
	defer func() {
		if err := application.Close(); err != nil {
			logger.Error("SentinelBox database close failed", "error", err)
		}
	}()

	if err := application.Run(ctx); err != nil {
		logger.Error("SentinelBox stopped with an error", "error", err)
		return 1
	}
	return 0
}
