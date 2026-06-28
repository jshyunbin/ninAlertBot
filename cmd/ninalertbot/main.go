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
	"time"

	"github.com/jshyunbin/ninalertbot/internal/config"
	"github.com/jshyunbin/ninalertbot/internal/monitor"
	"github.com/jshyunbin/ninalertbot/internal/notifier"
	"github.com/jshyunbin/ninalertbot/internal/state"
	"github.com/jshyunbin/ninalertbot/internal/store"
	"github.com/jshyunbin/ninalertbot/internal/updater"
)

// version is the build version, set via -ldflags "-X main.version=vX.Y.Z" by
// release.sh. Plain `go build` leaves it as "dev".
var version = "dev"

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	statePath := flag.String("state", "state.json", "path to state file")
	debug := flag.Bool("debug", false, "enable debug logging")
	checkOnce := flag.Bool("once", false, "check all products once and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	checkUpdate := flag.Bool("check-update", false, "check whether a newer release exists and exit")
	doUpdate := flag.Bool("update", false, "update to the latest release in place and exit")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	switch {
	case *showVersion:
		fmt.Printf("ninalertbot %s\n", version)
		return
	case *checkUpdate:
		if err := runCheckUpdate(); err != nil {
			log.Error("check-update failed", "err", err)
			os.Exit(1)
		}
		return
	case *doUpdate:
		if err := runUpdate(); err != nil {
			log.Error("update failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if err := run(*configPath, *statePath, *checkOnce, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func runCheckUpdate() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := updater.New(version).Check(ctx)
	if err != nil {
		return err
	}
	if res.HasUpdate {
		fmt.Printf("update available: %s -> %s\n%s\nrun `ninalertbot -update` to install\n",
			res.Current, res.Latest, res.URL)
	} else {
		fmt.Printf("up to date (%s)\n", res.Current)
	}
	return nil
}

func runUpdate() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := updater.New(version).Update(ctx, os.Stdout)
	if err != nil {
		return err
	}
	if !res.HasUpdate {
		fmt.Printf("already up to date (%s)\n", res.Current)
		return nil
	}
	fmt.Printf("updated %s -> %s. Restart ninAlertBot (or its service) to run the new version.\n",
		res.Current, res.Latest)
	return nil
}

// notifyIfUpdateAvailable does a best-effort, short-timeout check for a newer
// release and logs a notice. It never blocks startup meaningfully or fails the
// run if GitHub is unreachable.
func notifyIfUpdateAvailable(log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	res, err := updater.New(version).Check(ctx)
	if err != nil {
		log.Debug("update check skipped", "err", err)
		return
	}
	if res.HasUpdate {
		log.Warn(fmt.Sprintf("a new version is available (%s → %s) — run `ninalertbot -update` to update",
			res.Current, res.Latest), "url", res.URL)
	}
}

func run(configPath, statePath string, checkOnce bool, log *slog.Logger) error {
	notifyIfUpdateAvailable(log)

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
