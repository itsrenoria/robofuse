<p align="center">
  <img src="assets/logo.png" alt="robofuse" width="400" />
</p>

<p align="center">
  <a href="https://github.com/itsrenoria/robofuse/stargazers"><img src="https://img.shields.io/github/stars/itsrenoria/robofuse?style=flat-square" alt="Stars"></a>
  <a href="https://github.com/itsrenoria/robofuse/issues"><img src="https://img.shields.io/github/issues/itsrenoria/robofuse?style=flat-square" alt="Issues"></a>
  <a href="https://github.com/itsrenoria/robofuse/blob/legacy-python/LICENSE"><img src="https://img.shields.io/github/license/itsrenoria/robofuse?style=flat-square" alt="License"></a>
  <a href="https://github.com/itsrenoria/robofuse"><img src="https://img.shields.io/badge/docker-ready-blue?style=flat-square" alt="Docker"></a>
</p>

<p align="center">
  A Python service that interacts with the Real-Debrid API to generate .strm files<br>for media players like Infuse, Jellyfin, and Emby.
</p>

> ⚠️ **Note**: This is the legacy Python version. The main branch now contains a complete Go rewrite with improved performance.

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Quick Start](#quick-start)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

robofuse connects to your Real-Debrid account and efficiently manages your media files by:

1. Retrieving your torrents and downloads
2. Repairing dead torrents when needed
3. Unrestricting links automatically
4. Generating .strm files for streamable content
5. Maintaining your library by updating or removing stale entries
6. Intelligently organizing media files based on parsed metadata

## Features

- **Efficient API Integration**: Smart rate limiting to prevent API throttling
- **Parallel Processing**: Fast operations with concurrent API requests
- **Smart Organization**: Automatic categorization of media into appropriate folders
- **Metadata Parsing**: Intelligent filename parsing for proper media organization
- **Watch Mode**: Continuous monitoring for new content
- **Caching System**: Reduces redundant API calls
- **Link Management**: Handles expired links and refreshes them automatically
- **Health Checks**: Ensures content integrity
- **Clean UI**: Colorful terminal interface with progress bars
- **Docker Support**: Run in containers for easy deployment
- **Background Services**: Deploy with systemd, launchd, or Docker
- **Log Rotation**: Built-in log management for continuous operation
- **Anime Detection**: Automatically identifies and categorizes anime content

## Quick Start

1. **Install robofuse**:
   ```bash
   git clone -b legacy-python https://github.com/itsrenoria/robofuse.git
   cd robofuse
   pip install -e .
   ```

2. **Configure your Real-Debrid API token** in the existing `config.json` file

3. **Run robofuse**:
   ```bash
   # Show help
   robofuse --help
   
   # Test with dry run first
   robofuse --debug dry-run
   
   # Run once to process all content
   robofuse run
   
   # Start watch mode for continuous monitoring
   robofuse watch
   
   # Watch mode with custom interval
   robofuse watch --interval 300
   ```

4. **Deploy for continuous operation** (optional):
   - See our [Deployment Guide](wiki/Deployment.md) for systemd, launchd, or Docker setup

## Documentation

📚 **Complete documentation is available in the [wiki folder](wiki/)**

### Quick Links:
- **[🏠 Home](wiki/Home.md)** - Documentation overview and navigation
- **[📦 Installation](wiki/Installation.md)** - Complete installation instructions and setup
- **[⚙️ Configuration](wiki/Configuration.md)** - API setup, settings, and metadata parsing
- **[🚀 Usage](wiki/Usage.md)** - Command reference and usage patterns
- **[🔧 Deployment](wiki/Deployment.md)** - Background services, Docker, and production deployment
- **[🛠️ Troubleshooting](wiki/Troubleshooting.md)** - Common issues and debugging solutions

> 💡 **Need help?** Start with the [Troubleshooting Guide](wiki/Troubleshooting.md) or [open an issue](https://github.com/itsrenoria/robofuse/issues) if you encounter problems.

## Contributing

Contributions to robofuse are welcome! Here's how you can help:

1. **Bug Reports**: Open an issue describing the bug with steps to reproduce
2. **Feature Requests**: Open an issue describing the new feature and why it would be useful
3. **Code Contributions**: Submit a pull request with your improvements
   - Fork the repository
   - Create a feature branch (`git checkout -b feature/amazing-feature`)
   - Commit your changes (`git commit -m 'Add some amazing feature'`)
   - Push to the branch (`git push origin feature/amazing-feature`)
   - Open a Pull Request

Please ensure your code follows the existing style and includes appropriate tests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.