package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

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
			}
			// Start HTTP server for health checks and metrics
			go func() {
				mux := http.NewServeMux()
				mux.Handle("/healthz", health.Handler())
				mux.Handle("/metrics", metrics.Handler())
				http.ListenAndServe("127.0.0.1:9090", mux)
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if rebuildOrganized && !dryRun {
		fmt.Println("Rebuilding organized library from scratch...")

		cwd, _ := os.Getwd()
		allowedBases := []string{cwd, "/data", "/config"}

		safeRemoveAll := func(path string) {
			if path == "" {
				return
			}
			absPath, err := filepath.Abs(path)
			if err != nil {
				fmt.Printf("WARNING: cannot resolve path %s — skipping\n", path)
				return
			}
			ok := false
			for _, base := range allowedBases {
				if strings.HasPrefix(absPath, base+string(filepath.Separator)) || absPath == base {
					ok = true
					break
				}
			}
			if !ok {
				fmt.Printf("WARNING: %s is outside safe paths — skipping rebuild for safety\n", absPath)
				return
			}
			fmt.Printf("  Removing: %s\n", path)
			os.RemoveAll(path)
		}

		safeRemoveAll(cfg.OrganizedDir)
		safeRemoveAll(cfg.TrackingFile)
	}

	result, err := service.Run(ctx, dryRun)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println("\nShutdown signal received — gracefully stopping...")
		} else {
			log.Error().Err(err).Msg("Sync failed")
		}
		service.WaitForProbes()
		log.Info().Msg("Shutdown complete")
		if !errors.Is(err, context.Canceled) {
			os.Exit(1)
		}
		return
	}
	service.WaitForProbes()
	log.Info().Msg("Shutdown complete")

	summary := sync.FormatSummary(result, sync.SummaryOptions{
		DryRun:     dryRun,
		IncludeOrg: cfg.PttRename && !dryRun,
	})
	log.Info().Msg(summary)
}

func runWatch(cfg *config.Config) {
	log := logger.Default()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	service := sync.New(cfg)
	if err := service.Watch(ctx); err != nil {
		log.Error().Err(err).Msg("Watch mode failed")
		service.WaitForProbes()
		os.Exit(1)
	}
	service.WaitForProbes()
	log.Info().Msg("Watch mode shut down gracefully")
}
