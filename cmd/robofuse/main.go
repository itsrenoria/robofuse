package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/sync"
)

const version = "1.0"

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
	printBanner()

	switch command {
	case "run":
		log.Info().Msg("Starting single sync run...")
		runSync(cfg, false)

	case "watch":
		log.Info().Msgf("Starting watch mode (interval: %ds)...", cfg.WatchModeInterval)
		runWatch(cfg)

	case "dry-run", "dryrun":
		log.Info().Msg("Starting dry run (no changes will be made)...")
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

	printSummary(result, dryRun)
}

func runWatch(cfg *config.Config) {
	log := logger.Default()
	
	service := sync.New(cfg)
	if err := service.Watch(); err != nil {
		log.Error().Err(err).Msg("Watch mode failed")
		os.Exit(1)
	}
}

func printSummary(result *sync.RunResult, dryRun bool) {
	log := logger.Default()
	
	mode := "Sync"
	if dryRun {
		mode = "Dry Run"
	}

	log.Info().Msg("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info().Msgf("                    %s Complete", mode)
	log.Info().Msg("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info().Msgf("  Duration:          %s", result.Duration.Round(time.Millisecond))
	log.Info().Msg("")
	log.Info().Msg("  Downloads:")
	log.Info().Msgf("    Total:           %d", result.TorrentsTotal)
	log.Info().Msgf("    Downloaded:      %d", result.TorrentsDownloaded)
	log.Info().Msgf("    Dead:            %d", result.TorrentsDead)
	log.Info().Msgf("    Repaired:        %d", result.TorrentsRepaired)
	log.Info().Msg("")
	log.Info().Msg("  Links:")
	log.Info().Msgf("    Unrestricted:    %d", result.LinksUnrestricted)
	log.Info().Msgf("    Failed:          %d", result.LinksFailed)
	log.Info().Msg("")
	log.Info().Msg("  STRM Files:")
	log.Info().Msgf("    Added:           %d", result.STRMAdded)
	log.Info().Msgf("    Updated:         %d", result.STRMUpdated)
	log.Info().Msgf("    Deleted:         %d", result.STRMDeleted)
	log.Info().Msgf("    Skipped:         %d", result.STRMSkipped)
	
	if result.OrgProcessed > 0 {
		log.Info().Msg("")
		log.Info().Msg("  Organized:")
		log.Info().Msgf("    Processed:       %d", result.OrgProcessed)
		log.Info().Msgf("    New:             %d", result.OrgNew)
		log.Info().Msgf("    Updated:         %d", result.OrgUpdated)
		log.Info().Msgf("    Deleted:         %d", result.OrgDeleted)
		if result.OrgErrors > 0 {
			log.Info().Msgf("    Errors:          %d", result.OrgErrors)
		}
	}
	
	log.Info().Msg("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
