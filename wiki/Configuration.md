# Configuration Guide

## Prerequisites

Complete the [Installation Guide](Installation.md) first to install robofuse.

## Initial Setup

The robofuse project comes with a `config.json` file already configured with default values. Simply edit this file to add your Real-Debrid API token and customize the settings as needed:

```json
{
    "token": "YOUR_RD_API_TOKEN",
    "output_dir": "./Library",
    "cache_dir": "./cache",
    "concurrent_requests": 32,
    "general_rate_limit": 60,
    "torrents_rate_limit": 25,
    "watch_mode_interval": 60,
    "repair_torrents": true,
    "use_ptt_parser": true
}
```

## Getting Your API Token

1. Log in to [Real-Debrid](https://real-debrid.com/)
2. Go to Account → My Account → API
3. Generate or copy your token

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `token` | Your Real-Debrid API token (required) | - |
| `output_dir` | Directory where .strm files will be created | `./Library` |
| `cache_dir` | Directory for caching data to minimize API calls | `./cache` |
| `concurrent_requests` | Maximum number of parallel API requests | 32 |
| `general_rate_limit` | Rate limit for general API requests per minute | 60 |
| `torrents_rate_limit` | Rate limit for torrents API requests per minute | 25 |
| `watch_mode_interval` | Interval in seconds between runs in watch mode | 60 |
| `repair_torrents` | Whether to attempt repair of dead torrents | true |
| `use_ptt_parser` | Whether to use PTT for parsing filenames | true |

## Metadata Parsing

When `use_ptt_parser` is enabled, robofuse uses the [PTT (Parsett)](https://github.com/dreulavelle/PTT) library to extract metadata from filenames. This provides:

- Better organized library structure
- TV shows organized by seasons  
- Movies organized with year information
- Anime detection and organization
- Clean filenames without release group tags
- Consistent naming conventions

### Example Folder Structure

#### With PTT Parser Enabled:
```
Library/
  ├── Movies/
  │   ├── Action Movie (2020)/
  │   │   └── Action Movie (2020) [2160p, BluRay REMUX].strm
  │   └── Sci-Fi Film (2019)/
  │       └── Sci-Fi Film (2019) [1080p, BluRay].strm
  ├── TV Shows/
  │   └── Drama Series/
  │       ├── Season 01/
  │       │   ├── Drama Series S01E01 [720p, BluRay].strm
  │       │   └── Drama Series S01E02 [720p, BluRay].strm
  │       └── Season 02/
  │           └── Drama Series S02E01 [720p, BluRay].strm
  └── Anime/
      └── Anime Series/
          └── Anime Series - 001 [1080p].strm
```

#### Without PTT Parser:
```
Library/
  ├── Action.Movie.2020.UHD.BluRay.2160p.HEVC.REMUX-GROUP/
  │   └── Action.Movie.2020.UHD.BluRay.2160p.HEVC.REMUX-GROUP.strm
  ├── Sci-Fi.Film.2019.1080p.BluRay.x264-GROUP/
  │   └── Sci-Fi.Film.2019.1080p.BluRay.x264-GROUP.strm
  ├── Drama.Series.S01.720p.BluRay.x265-GROUP/
  │   ├── Drama.Series.S01E01.720p.BluRay.x265-GROUP.strm
  │   └── Drama.Series.S01E02.720p.BluRay.x265-GROUP.strm
  ├── Drama.Series.S02.720p.BluRay.x265-GROUP/
  │   └── Drama.Series.S02E01.720p.BluRay.x265-GROUP.strm
  └── Anime.Series.001.1080p.WEB-DL-GROUP/
      └── Anime.Series.001.1080p.WEB-DL-GROUP.strm
```

## What's Next

Choose your next step based on how you plan to use robofuse:

- **Test or run manually**: See the [Usage Guide](Usage.md) to learn commands and test your setup
- **Deploy as background service or Docker**: Skip to the [Deployment Guide](Deployment.md)
- **Need help with issues**: See the [Troubleshooting Guide](Troubleshooting.md)