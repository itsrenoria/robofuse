package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/sync"
)

const version = "1.1"

func main() {
	// Define flags
	var (
		configPath string
		logLevel   string
		showHelp   bool
		showVer    bool
	)

	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&configPath, "c", "", "Path to config file (shorthand)")
	flag.StringVar(&logLevel, "log-level", "", "Log level (debug, info, warn, error)")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showHelp, "h", false, "Show help (shorthand)")
	flag.BoolVar(&showVer, "version", false, "Show version")
	flag.BoolVar(&showVer, "v", false, "Show version (shorthand)")

	flag.Parse()

	if showVer {
		fmt.Printf("robofuse v%s\n", version)
		os.Exit(0)
	}

	if showHelp || len(flag.Args()) == 0 {
		printUsage()
		os.Exit(0)
	}

	command := strings.ToLower(flag.Arg(0))

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Set up logging
	if logLevel != "" {
		logger.SetLogLevel(logLevel)
	} else if cfg.LogLevel != "" {
		logger.SetLogLevel(cfg.LogLevel)
	}
	logger.SetLogPath(cfg.CacheDir)
	config.SetInstance(cfg)

	log := logger.Default()

	// Print banner
	if logger.IsTTY() && logger.IsInfoEnabled() {
		printBanner()
	}

	switch command {
	case "run":
		log.Info().Msg("run | mode=once dry=false")
		if logger.IsInfoEnabled() && logger.IsTTY() {
			fmt.Println()
		}
		runSync(cfg, false)

	case "watch":
		log.Info().Msgf("run | mode=watch interval=%ds", cfg.WatchModeInterval)
		if logger.IsInfoEnabled() && logger.IsTTY() {
			fmt.Println()
		}
		runWatch(cfg)

	case "dry-run", "dryrun":
		log.Info().Msg("run | mode=once dry=true")
		if logger.IsInfoEnabled() && logger.IsTTY() {
			fmt.Println()
		}
		runSync(cfg, true)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printBanner() {
	banner := `
  ██████╗  ██████╗ ██████╗  ██████╗ ███████╗██╗   ██╗███████╗███████╗
  ██╔══██╗██╔═══██╗██╔══██╗██╔═══██╗██╔════╝██║   ██║██╔════╝██╔════╝
  ██████╔╝██║   ██║██████╔╝██║   ██║█████╗  ██║   ██║███████╗█████╗  
  ██╔══██╗██║   ██║██╔══██╗██║   ██║██╔══╝  ██║   ██║╚════██║██╔══╝  
  ██║  ██║╚██████╔╝██████╔╝╚██████╔╝██║     ╚██████╔╝███████║███████╗
  ╚═╝  ╚═╝ ╚═════╝ ╚═════╝  ╚═════╝ ╚═╝      ╚═════╝ ╚══════╝╚══════╝
                                                          v` + version + `
`
	fmt.Println(banner)
}

func printUsage() {
	fmt.Printf(`robofuse v%s - Real-Debrid STRM file generator

Usage: robofuse [options] <command>

Commands:
  run       Run sync once and exit
  watch     Run sync continuously in watch mode
  dry-run   Show what would happen without making changes

Options:
  -c, --config <path>    Path to config file
  --log-level <level>    Log level (debug, info, warn, error)
  -v, --version          Show version
  -h, --help             Show this help

Examples:
  robofuse run
  robofuse --config /path/to/config.json watch
  robofuse dry-run
`, version)
}

func runSync(cfg *config.Config, dryRun bool) {
	log := logger.Default()

	service := sync.New(cfg)
	result, err := service.Run(dryRun)
	if err != nil {
		log.Error().Err(err).Msg("Sync failed")
		os.Exit(1)
	}

	summary := sync.FormatSummary(result, sync.SummaryOptions{
		DryRun:     dryRun,
		IncludeOrg: cfg.PttRename && !dryRun,
	})

	if logger.IsInfoEnabled() {
		if logger.IsTTY() {
			fmt.Println()
		}
		log.Info().Msg(summary)
	} else {
		if logger.IsTTY() {
			fmt.Println()
		}
		switch logger.GetLogLevel() {
		case "error":
			log.Error().Msg(summary)
		case "warn":
			log.Warn().Msg(summary)
		default:
			log.Info().Msg(summary)
		}
	}
}

func runWatch(cfg *config.Config) {
	log := logger.Default()

	service := sync.New(cfg)
	if err := service.Watch(); err != nil {
		log.Error().Err(err).Msg("Watch mode failed")
		os.Exit(1)
	}
}
