package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/sync"
)

const version = "1.0"

func main() {
	// Global flags
	var configPath string
	var verbose bool

	// Define subcommands
	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	runCmd.StringVar(&configPath, "config", "", "Path to config file")
	runCmd.BoolVar(&verbose, "verbose", false, "Verbose output")

	watchCmd := flag.NewFlagSet("watch", flag.ExitOnError)
	watchCmd.StringVar(&configPath, "config", "", "Path to config file")
	watchCmd.BoolVar(&verbose, "verbose", false, "Verbose output")

	dryRunCmd := flag.NewFlagSet("dry-run", flag.ExitOnError)
	dryRunCmd.StringVar(&configPath, "config", "", "Path to config file")
	dryRunCmd.BoolVar(&verbose, "verbose", false, "Verbose output")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runCmd.Parse(os.Args[2:])
		executeRun(configPath, verbose, false)

	case "watch":
		watchCmd.Parse(os.Args[2:])
		executeWatch(configPath, verbose)

	case "dry-run":
		dryRunCmd.Parse(os.Args[2:])
		executeRun(configPath, verbose, true)

	case "version", "-v", "--version":
		fmt.Printf("robofuse v%s\n", version)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Printf("Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`robofuse v%s - Real-Debrid STRM Generator

Usage:
  robofuse <command> [options]

Commands:
  run        Run sync once
  watch      Run in watch mode (continuous)
  dry-run    Show what would happen without writing
  version    Show version

Options:
  --config   Path to config.json (default: ./config.json)
  --verbose  Verbose output

Examples:
  robofuse run
  robofuse run --config /path/to/config.json
  robofuse watch --verbose
  robofuse dry-run

`, version)
}

func executeRun(configPath string, verbose, dryRun bool) {
	cfg := loadConfig(configPath, verbose)

	log := logger.Default()

	// Check if watch mode is enabled in config
	if cfg.WatchMode && !dryRun {
		log.Info().
			Str("version", version).
			Int("interval", cfg.WatchModeInterval).
			Msg("Watch mode enabled in config, starting...")

		syncService := sync.New(cfg)
		if err := syncService.Watch(); err != nil {
			log.Fatal().Err(err).Msg("Watch mode failed")
		}
		return
	}

	log.Info().
		Str("version", version).
		Msg("Robofuse starting...")

	syncService := sync.New(cfg)
	result, err := syncService.Run(dryRun)
	if err != nil {
		log.Fatal().Err(err).Msg("Sync failed")
	}

	// Print summary
	fmt.Println()
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println("Sync Summary")
	fmt.Printf("  Torrents: %d (%d dead, %d repaired)\n",
		result.TorrentsTotal, result.TorrentsDead, result.TorrentsRepaired)
	fmt.Printf("  Downloads: %d cached\n", result.DownloadsTotal)
	if result.LinksUnrestricted > 0 || result.LinksFailed > 0 {
		fmt.Printf("  Links: %d unrestricted, %d failed\n", result.LinksUnrestricted, result.LinksFailed)
	}
	strmTotal := result.STRMAdded + result.STRMUpdated + result.STRMSkipped
	fmt.Printf("  STRM: %d tracked (+%d/-%d/~%d)\n", strmTotal, result.STRMAdded, result.STRMDeleted, result.STRMUpdated)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))
	fmt.Println("─────────────────────────────────────────────")
	if result.OrgProcessed > 0 {
		fmt.Println("Organization Summary")
		fmt.Printf("  Files: %d (+%d/-%d/~%d)\n", result.OrgProcessed, result.OrgNew, result.OrgDeleted, result.OrgUpdated)
		if result.OrgErrors > 0 {
			fmt.Printf("  Errors: %d\n", result.OrgErrors)
		}
		fmt.Println("─────────────────────────────────────────────")
	}

	if dryRun {
		fmt.Println("\n[DRY-RUN] No changes were made")
	}
}

func executeWatch(configPath string, verbose bool) {
	cfg := loadConfig(configPath, verbose)

	// Force watch mode
	cfg.WatchMode = true

	log := logger.Default()
	log.Info().
		Str("version", version).
		Int("interval", cfg.WatchModeInterval).
		Msg("Watch mode starting...")

	syncService := sync.New(cfg)
	if err := syncService.Watch(); err != nil {
		log.Fatal().Err(err).Msg("Watch mode failed")
	}
}

func loadConfig(configPath string, verbose bool) *config.Config {
	// Set log level
	if verbose {
		logger.SetLogLevel("debug")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		fmt.Println("\nMake sure config.json exists with your Real-Debrid API token.")
		os.Exit(1)
	}

	// Set log path based on config
	logger.SetLogPath(cfg.CacheDir)

	// Update log level from config if not overridden by --verbose
	if !verbose {
		logger.SetLogLevel(cfg.LogLevel)
	}

	config.SetInstance(cfg)
	return cfg
}
