package repair

import (
	"context"
	"fmt"
	"time"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/rs/zerolog"
)

// repair.go re-adds dead torrents using cached magnet data when possible.

// Service handles torrent repair operations
type Service struct {
	rd     *realdebrid.Client
	config *config.Config
	logger zerolog.Logger
}

// New creates a new repair service
func New(rd *realdebrid.Client, cfg *config.Config) *Service {
	return &Service{
		rd:     rd,
		config: cfg,
		logger: logger.New("repair"),
	}
}

// RepairTorrent attempts to repair a dead/failed torrent by reinserting via magnet
func (s *Service) RepairTorrent(ctx context.Context, torrent *realdebrid.Torrent, dryRun bool) error {
	s.logger.Info().
		Str("id", torrent.ID).
		Str("filename", torrent.Filename).
		Str("hash", torrent.Hash[:8]).
		Msg("Repairing torrent")

	if dryRun {
		s.logger.Info().Msg("[DRY-RUN] Would repair torrent")
		return nil
	}

	// Step 1: Add magnet
	newID, err := s.rd.AddMagnet(ctx, torrent.Hash)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("adding magnet: %w", err)
	}
	s.logger.Debug().Str("newId", newID).Msg("Added magnet for repair")

	// Step 2: Wait for file list and select video files
	count, err := s.rd.SelectVideoFiles(ctx, newID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.rd.DeleteTorrent(cleanupCtx, newID)
			return ctxErr
		}
		// Clean up the new torrent if selection fails
		s.rd.DeleteTorrent(ctx, newID)
		return fmt.Errorf("selecting video files: %w", err)
	}
	s.logger.Debug().Int("files", count).Msg("Selected video files")

	// Step 3: Delete the original dead torrent
	if err := s.rd.DeleteTorrent(ctx, torrent.ID); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to delete original torrent")
		// Don't return error - the repair was successful
	}

	s.logger.Info().
		Str("oldId", torrent.ID).
		Str("newId", newID).
		Msg("Torrent repaired successfully")

	return nil
}

// RepairTorrents repairs multiple torrents
func (s *Service) RepairTorrents(ctx context.Context, torrents []*realdebrid.Torrent, dryRun bool) (int, int) {
	if len(torrents) == 0 {
		return 0, 0
	}

	s.logger.Info().Int("count", len(torrents)).Msg("Starting torrent repairs")

	var succeeded, failed int
	for _, t := range torrents {
		if err := ctx.Err(); err != nil {
			s.logger.Error().Err(err).Msg("Repair cancelled")
			failed += len(torrents) - succeeded - failed
			break
		}
		if err := s.RepairTorrent(ctx, t, dryRun); err != nil {
			s.logger.Error().Err(err).Str("filename", t.Filename).Msg("Repair failed")
			failed++
		} else {
			succeeded++
		}
	}

	s.logger.Info().
		Int("succeeded", succeeded).
		Int("failed", failed).
		Msg("Torrent repairs completed")

	return succeeded, failed
}

// RepairTorrentByHash repairs a torrent using just its hash
func (s *Service) RepairTorrentByHash(ctx context.Context, hash string, dryRun bool) error {
	if dryRun {
		s.logger.Info().Str("hash", hash[:8]).Msg("[DRY-RUN] Would repair torrent by hash")
		return nil
	}

	// Add magnet
	newID, err := s.rd.AddMagnet(ctx, hash)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("adding magnet: %w", err)
	}

	// Select video files
	count, err := s.rd.SelectVideoFiles(ctx, newID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.rd.DeleteTorrent(cleanupCtx, newID)
			return ctxErr
		}
		s.rd.DeleteTorrent(ctx, newID)
		return fmt.Errorf("selecting video files: %w", err)
	}

	s.logger.Info().
		Str("hash", hash[:8]).
		Str("id", newID).
		Int("files", count).
		Msg("Torrent added by hash")

	return nil
}
