# Usage Guide

## Prerequisites

1. Complete the [Installation Guide](Installation.md) to install robofuse
2. Complete the [Configuration Guide](Configuration.md) to set up your API token

## Basic Commands

### Run Once

```bash
robofuse run
```
Processes all content once.

### Watch Mode

```bash
robofuse watch
```
Runs in continuous watch mode. Uses the `watch_mode_interval` from config.json (default: 60 seconds).

```bash
robofuse watch --interval 300  # Custom interval (5 minutes)
```
Overrides the default interval and runs in watch mode with a custom 5-minute interval.

### Dry Run

```bash
robofuse dry-run
```
Test mode - shows what would happen without making any changes.

## Verbosity and Logging

Control log output with verbosity options. **Note**: Verbosity options must come BEFORE the command.

### Verbosity Levels

| Level | Description |
|-------|-------------|
| `debug` | Most detailed (for debugging issues) |
| `verbose` | Extra operational details |
| `info` | Normal output (default) |
| `warning` | Only warnings and errors |
| `error` | Only errors |

### Verbosity Options

```bash
# Using --verbosity flag
robofuse --verbosity debug run
robofuse -v verbose watch

# Using individual flags  
robofuse --debug run
robofuse --verbose watch
robofuse --info dry-run
robofuse --warning watch
robofuse --error run
```

## Configuration Options

### Custom Configuration File

You can specify a custom config file location:

```bash
robofuse --config /path/to/config.json run
```

### Combining Options

```bash
robofuse --config /path/to/config.json --debug watch --interval 600
```

## Command Reference

| Command | Description |
|---------|-------------|
| `run` | Process content once |
| `watch` | Continuous monitoring |
| `dry-run` | Test mode (no changes) |

## Global Options

| Option | Short | Description |
|--------|-------|-------------|
| `--verbosity LEVEL` | `-v LEVEL` | Set logging level |
| `--debug` | - | Debug logging |
| `--verbose` | - | Verbose logging |
| `--info` | - | Info logging |
| `--warning` | - | Warning logging |
| `--error` | - | Error logging |
| `--config PATH` | `-c PATH` | Custom config file |
| `--version` | - | Show version |
| `--help` | - | Show help |

## What's Next

- **Deploy as background service**: Go to the [Deployment Guide](Deployment.md) to set up robofuse to run automatically
- **Run manually**: Use the commands above when needed
- **Production deployment with Docker**: See the [Deployment Guide](Deployment.md) for Docker setup
- **Encountered issues**: See the [Troubleshooting Guide](Troubleshooting.md) for solutions
- **Customize configuration**: Return to the [Configuration Guide](Configuration.md) to adjust settings