// Command ninalertbot polls Nintendo Korea store products and sends a Discord
// alert when one becomes available to buy.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jshyunbin/ninalertbot/internal/config"
	"github.com/jshyunbin/ninalertbot/internal/monitor"
	"github.com/jshyunbin/ninalertbot/internal/notifier"
	"github.com/jshyunbin/ninalertbot/internal/state"
	"github.com/jshyunbin/ninalertbot/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	statePath := flag.String("state", "state.json", "path to state file")
	debug := flag.Bool("debug", false, "enable debug logging")
	checkOnce := flag.Bool("once", false, "check all products once and exit")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	if err := run(*configPath, *statePath, *checkOnce, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(configPath, statePath string, checkOnce bool, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := state.NewFileStore(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	checker := store.NewHTTPChecker()
	notif := notifier.NewDiscordWebhook(cfg.DiscordWebhookURL)
	mon := monitor.New(cfg, checker, checker, notif, st, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if checkOnce {
		mon.RunOnce(ctx)
		return nil
	}
	mon.Run(ctx)
	return nil
}
