package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/health"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/internal/metrics"
	"github.com/robofuse/robofuse/pkg/sync"
	"github.com/spf13/cobra"
)

var version = "2.0.0-dev"

var (
	cfgPath          string
	logLevel         string
	rebuildOrganized bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "robofuse",
		Short: "Real-Debrid STRM file generator",
		Long: `robofuse generates .strm files from Real-Debrid torrents for use
with media players like Infuse, Jellyfin, and Emby.`,
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if logLevel != "" {
				logger.SetLogLevel(logLevel)
			} else if cfg, err := config.Load(cfgPath); err == nil && cfg.LogLevel != "" {
				logger.SetLogLevel(cfg.LogLevel)
			}
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/healthz", health.Handler())
			mux.Handle("/metrics", metrics.Handler())
			if err := http.ListenAndServe("127.0.0.1:9090", mux); err != nil {
				fmt.Fprintf(os.Stderr, "Observability server failed: %v\n", err)
				os.Exit(1)
			}
		}()
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "Path to config file")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&rebuildOrganized, "rebuild-organized", false, "Delete organized directory and tracking database to force a full rebuild")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run sync once and exit",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			config.SetInstance(cfg)
			printBanner()
			runSync(cfg, false)
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "watch",
		Short: "Run sync continuously",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			config.SetInstance(cfg)
			printBanner()
			runWatch(cfg)
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "dry-run",
		Short: "Preview changes without making them",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			config.SetInstance(cfg)
			printBanner()
			runSync(cfg, true)
			return nil
		},
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
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

func runSync(cfg *config.Config, dryRun bool) {
	log := logger.Default()
	service := sync.New(cfg)

	if rebuildOrganized && !dryRun {
		fmt.Println("Rebuilding organized library from scratch...")

		var allowedBases []string
		if cwd, err := os.Getwd(); err == nil {
			allowedBases = append(allowedBases, cwd)
		}
		allowedBases = append(allowedBases, "/data", "/config")

		safeRemoveAll := func(path string) error {
			if path == "" {
				return nil
			}
			absPath, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("cannot resolve path %s: %w", path, err)
			}
			ok := false
			for _, base := range allowedBases {
				if strings.HasPrefix(absPath, base+string(filepath.Separator)) {
					ok = true
					break
				}
			}
			if !ok {
				return fmt.Errorf("%s is outside safe paths — refusing to delete for safety", absPath)
			}
			fmt.Printf("  Removing: %s\n", path)
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("failed to remove %s: %w", path, err)
			}
			return nil
		}

		if err := safeRemoveAll(cfg.OrganizedDir); err != nil {
			fmt.Fprintf(os.Stderr, "Rebuild error: %v\n", err)
			os.Exit(1)
		}
		if err := safeRemoveAll(cfg.TrackingFile); err != nil {
			fmt.Fprintf(os.Stderr, "Rebuild error: %v\n", err)
			os.Exit(1)
		}
	}

	result, err := service.Run(dryRun)
	if err != nil {
		log.Error().Err(err).Msg("Sync failed")
		os.Exit(1)
	}

	summary := sync.FormatSummary(result, sync.SummaryOptions{
		DryRun:     dryRun,
		IncludeOrg: cfg.PttRename && !dryRun,
	})
	log.Info().Msg(summary)
}

func runWatch(cfg *config.Config) {
	if rebuildOrganized {
		fmt.Fprintln(os.Stderr, "--rebuild-organized is only valid with 'run' or 'dry-run'")
		os.Exit(1)
	}

	log := logger.Default()

	service := sync.New(cfg)
	if err := service.Watch(); err != nil {
		log.Error().Err(err).Msg("Watch mode failed")
		os.Exit(1)
	}
	log.Info().Msg("Watch mode shut down gracefully")
}
