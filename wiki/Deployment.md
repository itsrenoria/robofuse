# Deployment Guide

## Prerequisites

1. Complete the [Installation Guide](Installation.md) - choose the appropriate method for your deployment type
2. Complete the [Configuration Guide](Configuration.md) to set up your API token  

**Note** For macOS/Linux deployment, you must complete the full installation including `pip install -e .`

**Note**: For Docker deployment, you only need to clone the repository (Docker handles the installation)

## Table of Contents

- [macOS (launchd)](#macos-launchd)
- [Linux (systemd)](#linux-systemd)
- [Docker Deployment](#docker-deployment)

## macOS (launchd)

> [!NOTE]
> **Choose the right guide for your installation:**
> - **Virtual Environment**: Continue with this guide if you used `python3 -m venv robofuse-env`
> - **Global Installation**: Expand the section below if you installed without virtual environment

<details>
<summary><h4>Global Installation (No Virtual Environment)</h4></summary>

**Prerequisites:**
- Completed installation with `pip install -e .` (no virtual environment)
- Verify robofuse works: `robofuse --version`

**Create Launch Agent:**

Create the launch agent directory:
```bash
mkdir -p ~/Library/LaunchAgents
```

Create the plist file (replace `USERNAME` with your actual macOS username):
```bash
cat > ~/Library/LaunchAgents/com.user.robofuse.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.robofuse</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>-c</string>
        <string>cd /Users/USERNAME/robofuse && robofuse watch</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>/Users/USERNAME/robofuse</string>
    <key>StandardOutPath</key>
    <string>/Users/USERNAME/Library/Logs/robofuse.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/USERNAME/Library/Logs/robofuse_error.log</string>
</dict>
</plist>
EOF
```

> [!CAUTION]
> You must replace `USERNAME` with your actual macOS username in **three places**:
> - The bash command path: `/Users/USERNAME/robofuse`
> - The WorkingDirectory: `/Users/USERNAME/robofuse`
> - Both log file paths: `/Users/USERNAME/Library/Logs/`

**Set Permissions and Load:**

Set correct permissions:
```bash
chmod 644 ~/Library/LaunchAgents/com.user.robofuse.plist
```

Create log directory:
```bash
mkdir -p ~/Library/Logs
```

Load and start the service:
```bash
launchctl load ~/Library/LaunchAgents/com.user.robofuse.plist
```

Start the service:
```bash
launchctl start com.user.robofuse
```

**Troubleshooting Global Installation:**

Test manually:
```bash
cd ~/robofuse
```

```bash
robofuse --verbose watch
```

Verify installation:
```bash
which robofuse
```

```bash
robofuse --version
```

</details>

### Virtual Environment Installation (Recommended)

#### Prerequisites for macOS Background Service

- macOS 10.15 (Catalina) or newer
- Completed installation with virtual environment (recommended)
- Your robofuse directory should be in your home folder: `~/robofuse`

#### Step 1: Create Launch Agent

Create the launch agent directory and file:

```bash
mkdir -p ~/Library/LaunchAgents
```

Create the plist file (replace `USERNAME` with your actual macOS username):

```bash
cat > ~/Library/LaunchAgents/com.user.robofuse.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.robofuse</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>-c</string>
        <string>cd /Users/USERNAME/robofuse && source robofuse-env/bin/activate && robofuse watch</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>/Users/USERNAME/robofuse</string>
    <key>StandardOutPath</key>
    <string>/Users/USERNAME/Library/Logs/robofuse.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/USERNAME/Library/Logs/robofuse_error.log</string>
</dict>
</plist>
EOF
```

> [!CAUTION]
> You must replace `USERNAME` with your actual macOS username in **three places**:
> - The bash command path: `/Users/USERNAME/robofuse`
> - The WorkingDirectory: `/Users/USERNAME/robofuse`  
> - Both log file paths: `/Users/USERNAME/Library/Logs/`

> [!NOTE]
> If robofuse is located in a different directory (not `~/robofuse`), update these paths in the plist:
> - `<string>cd /Users/USERNAME/your-robofuse-path && source robofuse-env/bin/activate && robofuse watch</string>`
> - `<string>/Users/USERNAME/your-robofuse-path</string>` (WorkingDirectory)
> - Log paths can remain in `~/Library/Logs/` or be changed to your preferred location

#### Step 2: Set Permissions and Load the Service

Set correct permissions:
```bash
chmod 644 ~/Library/LaunchAgents/com.user.robofuse.plist
```

Create log directory:
```bash
mkdir -p ~/Library/Logs
```

Load and start the service:
```bash
launchctl load ~/Library/LaunchAgents/com.user.robofuse.plist
```

#### Managing the macOS Service

**Check service status:**
```bash
launchctl list | grep robofuse
```

**View logs:**

View current log:
```bash
cat ~/Library/Logs/robofuse.log
```

Monitor logs in real-time:
```bash
tail -f ~/Library/Logs/robofuse.log
```

Check for errors:
```bash
cat ~/Library/Logs/robofuse_error.log
```

**Stop/Start/Restart service:**

Stop service:
```bash
launchctl unload ~/Library/LaunchAgents/com.user.robofuse.plist
```

Start service:
```bash
launchctl load ~/Library/LaunchAgents/com.user.robofuse.plist
```

Restart service (after updates):
```bash
launchctl unload ~/Library/LaunchAgents/com.user.robofuse.plist
launchctl load ~/Library/LaunchAgents/com.user.robofuse.plist
```

**Update robofuse:**

To update robofuse to the latest version:

```bash
# Stop the service
launchctl unload ~/Library/LaunchAgents/com.user.robofuse.plist

# Navigate to robofuse directory and update
cd ~/robofuse
git pull origin

# Activate virtual environment and reinstall
source robofuse-env/bin/activate
pip install -e .

# Restart the service
launchctl load ~/Library/LaunchAgents/com.user.robofuse.plist
```

**Remove service completely:**
```bash
# Stop and remove
launchctl unload ~/Library/LaunchAgents/com.user.robofuse.plist
rm ~/Library/LaunchAgents/com.user.robofuse.plist
rm ~/Library/Logs/robofuse.log ~/Library/Logs/robofuse_error.log
```

> [!NOTE]
> If you used different log file paths in your plist, update the `rm` command accordingly to remove your custom log files.

#### Log Rotation for macOS

As robofuse runs continuously, log files can grow large over time. Here's how to set up automatic log rotation:

**Option 1: Using logrotate (if installed via Homebrew)**

Install logrotate:
```bash
brew install logrotate
```

Create a logrotate configuration:
```bash
mkdir -p ~/.config/logrotate
cat > ~/.config/logrotate/robofuse << 'EOF'
/Users/*/Library/Logs/robofuse.log /Users/*/Library/Logs/robofuse_error.log {
    daily
    missingok
    rotate 7
    compress
    delaycompress
    copytruncate
    create 644
}
EOF
```

Add to crontab for daily rotation:
```bash
(crontab -l 2>/dev/null; echo "0 2 * * * /usr/local/bin/logrotate ~/.config/logrotate/robofuse") | crontab -
```

**Option 2: Simple manual rotation script**

Create a rotation script:
```bash
cat > ~/Library/Scripts/rotate_robofuse_logs.sh << 'EOF'
#!/bin/bash
LOG_DIR="$HOME/Library/Logs"
MAX_SIZE=10485760  # 10MB in bytes

for log_file in "$LOG_DIR/robofuse.log" "$LOG_DIR/robofuse_error.log"; do
    if [[ -f "$log_file" ]] && [[ $(stat -f%z "$log_file" 2>/dev/null || echo 0) -gt $MAX_SIZE ]]; then
        # Backup current log
        mv "$log_file" "${log_file}.$(date +%Y%m%d_%H%M%S)"
        # Create new empty log
        touch "$log_file"
        # Keep only last 5 backups
        find "$LOG_DIR" -name "$(basename "$log_file").*" -type f | sort | head -n -5 | xargs rm -f
    fi
done
EOF
```

Make it executable and add to crontab:
```bash
chmod +x ~/Library/Scripts/rotate_robofuse_logs.sh
mkdir -p ~/Library/Scripts
(crontab -l 2>/dev/null; echo "0 2 * * * ~/Library/Scripts/rotate_robofuse_logs.sh") | crontab -
```

**Manual log cleanup:**
```bash
# Archive current logs and start fresh
cd ~/Library/Logs
mv robofuse.log robofuse.log.$(date +%Y%m%d_%H%M%S)
mv robofuse_error.log robofuse_error.log.$(date +%Y%m%d_%H%M%S)
touch robofuse.log robofuse_error.log
```

#### Troubleshooting macOS Background Service

**If logs are empty or service isn't working:**

1. **Check if service is loaded and running:**
   ```bash
   launchctl list | grep robofuse
   ```
   You should see output like: `12345  0  com.user.robofuse`

2. **Test the command manually:**
   ```bash
   cd ~/robofuse
   source robofuse-env/bin/activate
   robofuse --verbose watch
   ```
   Press Ctrl+C to stop. If this works, the issue is with the launch agent configuration.

3. **Check if USERNAME was properly replaced:**
   ```bash
   cat ~/Library/LaunchAgents/com.user.robofuse.plist | grep USERNAME
   ```
   This should return nothing. If you see "USERNAME", edit the file and replace with your actual username.

4. **Verify paths exist:**
   ```bash
   ls -la ~/robofuse/robofuse-env/bin/activate
   ls -la ~/Library/Logs/
   which robofuse  # After activating virtual environment
   ```

5. **Check for permission issues:**
   ```bash
   # Make sure virtual environment is executable
   chmod +x ~/robofuse/robofuse-env/bin/activate
   ```

6. **If still not working, try reloading:**
   ```bash
   launchctl unload ~/Library/LaunchAgents/com.user.robofuse.plist
   launchctl load ~/Library/LaunchAgents/com.user.robofuse.plist
   ```

7. **Check system logs for launch agent errors:**
   ```bash
   log show --predicate 'process == "launchd"' --last 30m | grep robofuse
   ```

## Linux (systemd)

1. Create a service file:
   ```bash
   sudo nano /etc/systemd/system/robofuse.service
   ```

2. Add the configuration:
   ```ini
   [Unit]
   Description=robofuse
   After=network.target

   [Service]
   Type=simple
   User=your_username
   WorkingDirectory=/path/to/robofuse
   ExecStart=/usr/bin/python3 -m robofuse watch
   Restart=on-failure
   RestartSec=5s

   [Install]
   WantedBy=multi-user.target
   ```

   > [!CAUTION]
   > You must replace the following in the configuration above:
   > - `your_username` - Replace with your actual Linux username
   > - `/path/to/robofuse` - Replace with the full path to your robofuse directory (e.g., `/home/yourusername/robofuse`)

3. Manage the service:
   ```bash
   # Enable and start
   sudo systemctl enable robofuse
   sudo systemctl start robofuse
   
   # Check status and logs
   sudo systemctl status robofuse
   sudo journalctl -u robofuse -f
   
   # Stop and restart
   sudo systemctl stop robofuse
   sudo systemctl restart robofuse
   ```

4. **Update robofuse:**

   To update robofuse to the latest version:

   ```bash
   # Stop the service
   sudo systemctl stop robofuse

   # Navigate to robofuse directory and update
   cd /path/to/robofuse  # Replace with your actual path
   git pull origin

   # If using virtual environment, activate it first
   # source robofuse-env/bin/activate

   # Reinstall robofuse
   pip install -e .

   # Start the service
   sudo systemctl start robofuse

   # Check that it's running properly
   sudo systemctl status robofuse
   ```

5. Remove service completely:
   ```bash
   # Stop and disable the service
   sudo systemctl stop robofuse
   sudo systemctl disable robofuse
   
   # Remove the service file
   sudo rm /etc/systemd/system/robofuse.service
   
   # Reload systemd to reflect changes
   sudo systemctl daemon-reload
   ```

## Docker Deployment

### Quick Start

1. Ensure `config.json` is configured with your API token
2. Run:
   ```bash
   docker compose up -d
   ```

### Configuration

The `docker-compose.yml` maps these volumes:
```yaml
volumes:
  - ./config.json:/app/config.json  # Configuration
  - ./cache:/app/cache              # Cache directory
  - ./Library:/app/Library          # Output directory
```

To use a different media location:
```yaml
volumes:
  - ./config.json:/app/config.json
  - ./cache:/app/cache
  - /path/to/your/media:/app/Library
```

### Docker Commands

**Start container:**
```bash
docker compose up -d
```

**View logs:**
```bash
docker compose logs -f
```

**Stop container:**
```bash
docker compose down
```

**Update and restart:**

> [!NOTE]
> To update to the latest version, navigate to your robofuse directory and pull the latest changes:

```bash
cd robofuse
```

```bash
git pull origin
```

```bash
docker compose pull && docker compose up -d
```

## What's Next

Your robofuse deployment is now running automatically. Here's what you can do:

- **Monitor operation**: Use your service's logging commands to check if everything is working properly
- **Test functionality**: Check if .strm files are being created in your Library directory
- **Adjust settings**: Return to [Configuration Guide](Configuration.md) to modify intervals, or other options
- **Troubleshoot issues**: See the [Troubleshooting Guide](Troubleshooting.md) if you encounter problems
- **Learn more commands**: Review the [Usage Guide](Usage.md) for manual operations, testing or learn about the commands

## Additional Tips

### Passing Additional Command Line Arguments

By default, the deployment configurations run `robofuse watch`, but you may want to pass additional arguments like `--verbose`, `--debug`, or other options. Here's how to modify each deployment method:

#### macOS (launchd)

To add additional arguments (like `--verbose` or `--debug`), edit your plist file:

```bash
nano ~/Library/LaunchAgents/com.user.robofuse.plist
```

Change the `ProgramArguments` section from:
```xml
<string>cd /Users/USERNAME/robofuse && source robofuse-env/bin/activate && robofuse watch</string>
```

To (example with verbose logging):
```xml
<string>cd /Users/USERNAME/robofuse && source robofuse-env/bin/activate && robofuse --verbose watch</string>
```

Then reload the service:
```bash
launchctl unload ~/Library/LaunchAgents/com.user.robofuse.plist
launchctl load ~/Library/LaunchAgents/com.user.robofuse.plist
```

#### Linux (systemd)

Edit your service file:
```bash
sudo nano /etc/systemd/system/robofuse.service
```

Change the `ExecStart` line from:
```ini
ExecStart=/usr/bin/python3 -m robofuse watch
```

To (example with verbose logging):
```ini
ExecStart=/usr/bin/python3 -m robofuse --verbose watch
```

Then reload and restart:
```bash
sudo systemctl daemon-reload
sudo systemctl restart robofuse
```

#### Docker

For Docker deployment, modify your `docker-compose.yml` file. Change the command section from:
```yaml
command: ["python", "-m", "robofuse", "watch"]
```

To (example with verbose logging):
```yaml
command: ["python", "-m", "robofuse", "--verbose", "watch"]
```

Then restart the container:
```bash
docker compose down
docker compose up -d
```

### Available Command Line Options

To see all available command line options and learn more about robofuse commands, check out the [Usage Guide](Usage.md). Common useful options include:

- `--verbose`: Detailed logging output
- `--debug`: Debug level logging with extra details
- `--config`: Specify a custom config file location
- `--cache-dir`: Use a custom cache directory
- `--output-dir`: Use a custom output directory

You can combine multiple options, for example: `robofuse --verbose --debug watch`