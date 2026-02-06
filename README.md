<p align="center">
  <img src="assets/logo.png" alt="robofuse" width="400" />
</p>

<p align="center">
  <a href="https://github.com/itsrenoria/robofuse/stargazers"><img src="https://img.shields.io/github/stars/itsrenoria/robofuse?style=flat-square" alt="Stars"></a>
  <a href="https://github.com/itsrenoria/robofuse/issues"><img src="https://img.shields.io/github/issues/itsrenoria/robofuse?style=flat-square" alt="Issues"></a>
  <a href="https://github.com/itsrenoria/robofuse/blob/main/LICENSE"><img src="https://img.shields.io/github/license/itsrenoria/robofuse?style=flat-square" alt="License"></a>
  <a href="https://github.com/itsrenoria/robofuse"><img src="https://img.shields.io/badge/docker-ready-blue?style=flat-square" alt="Docker"></a>
</p>

# robofuse v1.0
> **A high-performance Real-Debrid STRM file generator for your media server.**

**robofuse** is a lightweight, blazing-fast service that interacts with the [Real-Debrid](https://real-debrid.com/) API to automatically organize your movie and TV library. It generates `.strm` files for use with media players like **Infuse**, **Jellyfin**, **Emby**, and **Plex**.

Rewritten from the ground up in **Go**, robofuse v1.0 is designed for speed, efficiency, and stability.

---

## ✨ Features

- 🚀 **Blazing Fast**: Built with Go's concurrent worker pools for maximum performance.
- 🔄 **Smart Sync**: Only updates what's changed. Adds new files, updates modified ones, and cleans up orphans.
- 🧹 **Auto-Repair**: Automatically detects dead downloads and re-adds them using cached magnet links.
- 📦 **Paginator**: Handles large libraries with ease by paginating through your Real-Debrid downloads.
- ⏱️ **Watch Mode**: Set it and forget it. Runs continuously in the background to keep your library fresh.
- 🛡️ **Rate Limit Protection**: Smartly respects separate rate limits for different API endpoints.
- 🎯 **Deduplication**: Automatically handles duplicate downloads, keeping only the latest version.
- 📝 **Metadata Parsing**: Integrated PTT logic for cleaner, better-organized file names.

---

## 🚀 Installation

### Prerequisites

- **Real-Debrid Account**: You need an API token from your [Real-Debrid Account Panel](https://real-debrid.com/apitoken).
- **Docker**: Required for running robofuse.

### Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/itsrenoria/robofuse.git
   cd robofuse
   ```

2. Configure your API token:
   ```bash
   # Edit config.json and replace YOUR_REAL_DEBRID_API_TOKEN with your actual token
   nano config.json
   ```

3. Build and start robofuse:
   ```bash
   docker compose up -d --build
   ```

4. View logs:
   ```bash
   docker compose logs -f
   ```

---

## ⚙️ Configuration

Edit `config.json` to customize robofuse:

### Configuration Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `token` | string | **Required** | Your Real-Debrid API Token. |
| `output_dir` | string | `./library` | Where raw STRM files will be generated. |
| `organized_dir` | string | `./library-organized` | Where renamed/organized STRM files will be placed if `ptt_rename` is set to `true`. |
| `cache_dir` | string | `./cache` | Directory for storing state/cache. |
| `concurrent_requests`* | int | `50` | Max concurrent worker threads for processing. |
| `general_rate_limit`* | int | `250` | Request limit per minute for general API calls. |
| `torrents_rate_limit` | int | `25` | Request limit per minute for download endpoints. |
| `watch_mode` | bool | `false` | If set to `false`, robofuse will only run a single sync cycle. If set to `true`, robofuse will run in watch mode, which will run continuously in the background syncing every `watch_mode_interval` seconds. |
| `watch_mode_interval` | int | `60` | Seconds to wait between sync cycles in watch mode. |
| `repair_torrents` | bool | `true` | Automatically attempt to repair dead downloads by re-adding magnets. |
| `min_file_size_mb` | int | `150` | Ignore files smaller than this size (prevents samples), subtitles are ignored. |
| `ptt_rename` | bool | `true` | Use PTT logic to clean/rename files. |
| `log_level` | string | `"info"` | Logging verbosity (`debug`, `info`, `warn`, `error`). |
| `file_expiry_days` | int | `6` | Days to consider a file as expired from downloads in real-debrid. |

\* These rate limits are conservative defaults that work well for large libraries. You can push higher (e.g. `600`+ for general, `100`+ concurrent) but YMMV depending on your library size and Real-Debrid's current load.

> [!IMPORTANT]
> Don't delete `library`, `library-organized`, or `cache` by hand. These folders are part of the state/tracking system. If you need to reset, stop the service, back up what you need, then clear them intentionally.

---

## 🎮 Usage

### Docker Compose

The included `docker-compose.yml` builds and runs robofuse:

```yaml
services:
  robofuse:
    build: .
    container_name: robofuse
    restart: unless-stopped
    volumes:
      - ./config.json:/data/config.json
      - ./library:/app/library
      - ./library-organized:/app/library-organized
      - ./cache:/app/cache
```

### Common Operations

```bash
# Start in background
docker compose up -d --build

# View logs
docker compose logs -f

# Stop
docker compose down

# Rebuild after code changes
docker compose up -d --build
```

---

## ❤️ Support the Project

robofuse is a passion project developed and maintained for free. If you find it useful, please consider supporting its development.

- ⭐ **Star the Repository** on GitHub
- 🤝 **Contribute**:
  - **Bug Reports**: Open an issue describing the bug with steps to reproduce
  - **Feature Requests**: Open an issue describing the new feature and why it would be useful
  - **Code Contributions**: Submit a pull request with your improvements

## 🙏 Credits

This project wouldn't be possible without the foundational work of the open-source community.

- **[PTT-Go](https://github.com/itsrenoria/PTT-Go)**: Our Go port of the excellent [dreulavelle/PTT](https://github.com/dreulavelle/PTT) filename parsing library.
- **[Decypharr](https://github.com/sirrobot01/decypharr)**: Portions of this codebase were inspired by or repurposed from Decypharr (Copyright (c) 2025 Mukhtar Akere), used under the MIT License.

---

*Not affiliated with Real-Debrid.*
