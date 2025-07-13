# CloudAWSync - Cloud File Synchronization Agent

CloudAWSync is a cloud file synchronization agent written in Go. It provides real-time and scheduled synchronization between local directories and cloud storage (currently AWS S3), with support for multiple sync modes, comprehensive monitoring, and security features.

## Features

### Core Functionality
- **Multi-mode Synchronization**: Real-time, scheduled, or hybrid sync modes
- **AWS S3 Support**: Full S3 integration with support for S3-compatible services
- **Modular Architecture**: Easy to extend for other cloud providers
- **SystemD Integration**: Designed to run as a system service

### Performance & Reliability
- **High Concurrency**: Configurable concurrent upload/download workers
- **Bandwidth Control**: Optional bandwidth limiting
- **Retry Logic**: Automatic retry with exponential backoff
- **Integrity Verification**: MD5 hash verification for all transfers
- **Efficient Batching**: Event batching to reduce redundant operations

### Monitoring & Metrics
- **Prometheus Integration**: Comprehensive metrics collection
- **System Monitoring**: CPU, memory, disk usage tracking
- **Transfer Statistics**: Bandwidth, file counts, error rates
- **Health Reporting**: Sync status and error reporting

### Security & Safety
- **Encryption Support**: Server-side encryption for S3
- **File Filtering**: Configurable include/exclude patterns
- **Path Validation**: Protection against path traversal attacks
- **Permission Preservation**: Maintains file permissions when possible

## Quick Start

### Installation Methods

#### Method 1: Automated Installation (Recommended)

CloudAWSync includes an installation script that handles system setup, user creation, and service configuration automatically:

```bash
# Clone the repository
git clone https://github.com/aaronlmathis/CloudAWSync.git
cd CloudAWSync

# Make the script executable
chmod +x install.sh

# Run automated installation
sudo ./install.sh install
```

#### Method 2: Manual Installation

1. **Download and Build**:
```bash
git clone https://github.com/aaronlmathis/CloudAWSync.git
cd CloudAWSync
go mod tidy
go build -o cloudawsync
```

2. **Generate Configuration**:
```bash
./cloudawsync -generate-config
```

3. **Edit Configuration**:
Edit `cloudawsync-config.yaml` to configure your AWS credentials and sync directories.

4. **Run the Service**:
```bash
# Test run (foreground)
./cloudawsync -daemon=false -log-level=debug

# Production run (background)
./cloudawsync
```

## Installation Script Reference

The `install.sh` script provides complete installation and management capabilities for CloudAWSync. It automates system user creation, directory setup, service installation, and maintenance tasks.

### Script Features

- **Automated Dependency Management**: Checks and installs Go if needed
- **System User Creation**: Creates dedicated `cloudawsync` user and group
- **Security Hardening**: Implements proper file permissions and systemd security
- **Service Management**: Full systemd service lifecycle management
- **Log Management**: Configures log rotation and bash completion
- **Easy Maintenance**: Built-in update, backup, and monitoring commands

### Basic Usage

```bash
sudo ./install.sh <command>
```

### Available Commands

#### Installation Commands

**`install` (default)** - Complete system installation
```bash
sudo ./install.sh install
# or simply
sudo ./install.sh
```

**What it does:**
- Checks for required dependencies (Go, systemctl)
- Installs Go 1.21 if not present
- Creates dedicated `cloudawsync` system user and group
- Creates directory structure:
  - `/opt/cloudawsync/` - Installation directory
  - `/etc/cloudawsync/` - Configuration directory  
  - `/var/log/cloudawsync/` - Log directory
- Builds CloudAWSync binary with optimization flags
- Generates and installs sample configuration
- Creates systemd service with security hardening
- Sets up log rotation via logrotate
- Installs bash completion
- Provides next-step instructions

#### Service Management Commands

**`status`** - Show comprehensive service status
```bash
sudo ./install.sh status
```
- Displays systemd service status
- Shows recent log entries (last 20 lines)
- No root privileges required for viewing

**`restart`** - Restart the CloudAWSync service
```bash
sudo ./install.sh restart
```
- Cleanly restarts the service
- Useful after configuration changes

**`logs`** - Follow service logs in real-time
```bash
sudo ./install.sh logs
```
- Equivalent to `journalctl -u cloudawsync -f`
- Press Ctrl+C to exit

#### Maintenance Commands

**`uninstall`** - Remove CloudAWSync
```bash
sudo ./install.sh uninstall
```

**What it removes:**
- Stops and disables the systemd service
- Removes systemd service file
- Removes logrotate configuration
- Removes bash completion
- Removes installation directory (`/opt/cloudawsync/`)
- Removes log directory (`/var/log/cloudawsync/`)
- Removes system user and group
- **Note**: Configuration files in `/etc/cloudawsync/` are preserved

**`help`** - Show usage information
```bash
./install.sh help
# or
./install.sh --help
./install.sh -h
```

### Installation Directory Structure

After installation, CloudAWSync uses this directory structure:

```
/opt/cloudawsync/              # Installation directory (owned by cloudawsync:cloudawsync)
└── cloudawsync                # Main binary (executable)

/etc/cloudawsync/              # Configuration directory (owned by root:root)
├── config.yaml                # Main configuration file
└── config.yaml.example        # Example configuration (if available)

/var/log/cloudawsync/          # Log directory (owned by cloudawsync:cloudawsync)
└── cloudawsync.log            # Application logs (rotated daily)

/etc/systemd/system/           # SystemD integration
└── cloudawsync.service        # Service definition

/etc/logrotate.d/              # Log rotation
└── cloudawsync                # Log rotation configuration

/etc/bash_completion.d/        # Shell completion
└── cloudawsync                # Bash completion script
```

### SystemD Service Configuration

The script creates a production-ready systemd service with security hardening:

```ini
[Unit]
Description=CloudAWSync - Cloud File Synchronization Agent
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cloudawsync
Group=cloudawsync
WorkingDirectory=/opt/cloudawsync
ExecStart=/opt/cloudawsync/cloudawsync -config=/etc/cloudawsync/config.yaml -daemon=true
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=cloudawsync

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/log/cloudawsync /etc/cloudawsync

# Resource limits
MemoryMax=512M
CPUQuota=50%

[Install]
WantedBy=multi-user.target
```

### Security Features

The installation script implements several security best practices:

- **Dedicated User**: Runs as non-privileged `cloudawsync` system user
- **Minimal Permissions**: Restrictive file permissions (755 for binary, 644 for config)
- **SystemD Security**: NoNewPrivileges, PrivateTmp, ProtectSystem restrictions
- **Resource Limits**: Memory and CPU quotas to prevent resource exhaustion
- **Secure Directories**: Proper ownership and permissions for all directories

### Example Installation Workflow

#### First-time Installation

```bash
# 1. Clone and prepare
git clone https://github.com/aaronlmathis/CloudAWSync.git
cd CloudAWSync
chmod +x install.sh

# 2. Install with all dependencies
sudo ./install.sh install

# 3. Configure AWS credentials and directories
sudo nano /etc/cloudawsync/config.yaml

# 4. Enable and start the service  
sudo systemctl enable cloudawsync
sudo systemctl start cloudawsync

# 5. Verify installation
sudo ./install.sh status
```

#### Post-Installation Configuration

After installation, you must configure CloudAWSync before starting:

```bash
# Edit the main configuration file
sudo nano /etc/cloudawsync/config.yaml
```

**Required configuration:**
- AWS credentials (access_key_id, secret_access_key)
- S3 bucket name
- Local directories to sync
- Sync modes and schedules

**Example minimal configuration:**
```yaml
aws:
  region: "us-east-1"
  s3_bucket: "your-backup-bucket"
  access_key_id: "YOUR_ACCESS_KEY_ID"
  secret_access_key: "YOUR_SECRET_ACCESS_KEY"

directories:
  - local_path: "/home/user/Documents"
    remote_path: "documents"
    sync_mode: "realtime"
    recursive: true
    enabled: true
```

#### Service Management

```bash
# Start the service
sudo systemctl start cloudawsync

# Check status
sudo ./install.sh status

# Follow logs
sudo ./install.sh logs

# Restart after config changes
sudo ./install.sh restart

# Stop the service
sudo systemctl stop cloudawsync
```

### Troubleshooting Installation

#### Permission Issues
```bash
# Verify installation directories exist with correct ownership
ls -la /opt/cloudawsync/
ls -la /etc/cloudawsync/
ls -la /var/log/cloudawsync/

# Fix ownership if needed
sudo chown -R cloudawsync:cloudawsync /opt/cloudawsync/
sudo chown -R cloudawsync:cloudawsync /var/log/cloudawsync/
```

#### Service Issues
```bash
# Check if service file exists
ls -la /etc/systemd/system/cloudawsync.service

# Reload systemd configuration
sudo systemctl daemon-reload

# Check service status
sudo systemctl status cloudawsync

# View detailed logs
sudo journalctl -u cloudawsync -n 50
```

#### Dependency Issues
```bash
# Verify Go installation
go version

# Check if binary was built correctly
/opt/cloudawsync/cloudawsync -version

# Test configuration generation
sudo -u cloudawsync /opt/cloudawsync/cloudawsync -generate-config
```

### Log Management

The script configures automatic log rotation:

```bash
# Log rotation configuration
cat /etc/logrotate.d/cloudawsync

# Manually rotate logs
sudo logrotate -f /etc/logrotate.d/cloudawsync

# View current log
sudo tail -f /var/log/cloudawsync/cloudawsync.log
```

### Bash Completion

After installation, bash completion is available for the CloudAWSync binary:

```bash
# Tab completion works for options
/opt/cloudawsync/cloudawsync -<TAB>

# Available completions: -config, -daemon, -generate-config, -help, -log-level, -version
```

### Uninstallation

To completely remove CloudAWSync:

```bash
# Stop and remove service
sudo ./install.sh uninstall

# Manual cleanup if needed (configuration files are preserved by default)
sudo rm -rf /etc/cloudawsync/
```

**Note**: The uninstall command preserves configuration files in `/etc/cloudawsync/` by design. Remove them manually if you want a complete cleanup.

## Configuration

The configuration file uses YAML format. Here's a minimal example:

```yaml
aws:
  region: "us-east-1"
  s3_bucket: "my-backup-bucket"
  s3_prefix: "cloudawsync/"
  access_key_id: "YOUR_ACCESS_KEY"
  secret_access_key: "YOUR_SECRET_KEY"

directories:
  - local_path: "/home/user/Documents"
    remote_path: "documents"
    sync_mode: "realtime"
    recursive: true
    enabled: true
    filters:
      - "*.tmp"
      - "*.lock"
      - ".DS_Store"

  - local_path: "/home/user/Pictures"
    remote_path: "pictures"
    sync_mode: "scheduled"
    schedule: "0 2 * * *"  # Daily at 2 AM
    recursive: true
    enabled: true

logging:
  level: "info"
  format: "json"
  output_path: "/var/log/cloudawsync/cloudawsync.log"

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

performance:
  max_concurrent_uploads: 5
  max_concurrent_downloads: 5
  upload_chunk_size: 5242880  # 5MB
  retry_attempts: 3
  retry_delay: "5s"
```

### SystemD Service

1. **Generate service file**:
```bash
./cloudawsync -generate-systemd
```

2. **Install service**:
```bash
sudo cp cloudawsync.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable cloudawsync
sudo systemctl start cloudawsync
```

3. **Check status**:
```bash
sudo systemctl status cloudawsync
journalctl -u cloudawsync -f
```

## Configuration Reference

### AWS Configuration
- `region`: AWS region (default: us-east-1)
- `s3_bucket`: S3 bucket name (required)
- `s3_prefix`: Prefix for all uploaded files
- `access_key_id`: AWS access key ID
- `secret_access_key`: AWS secret access key
- `session_token`: AWS session token (optional)
- `endpoint`: Custom S3 endpoint for S3-compatible services

### Directory Configuration
- `local_path`: Local directory to sync (absolute path required)
- `remote_path`: Remote path in S3 bucket
- `sync_mode`: "realtime", "scheduled", or "both"
- `schedule`: Cron expression for scheduled sync
- `recursive`: Sync subdirectories recursively
- `enabled`: Enable/disable this directory
- `filters`: File patterns to exclude

### Performance Tuning
- `max_concurrent_uploads`: Number of simultaneous uploads
- `max_concurrent_downloads`: Number of simultaneous downloads
- `upload_chunk_size`: Chunk size for multipart uploads
- `retry_attempts`: Number of retry attempts on failure
- `retry_delay`: Delay between retries
- `bandwidth_limit`: Bandwidth limit in bytes/second (0 = unlimited)

### Security Settings
- `encryption_enabled`: Enable S3 server-side encryption
- `max_file_size`: Maximum file size to sync
- `allowed_extensions`: Whitelist of file extensions
- `denied_extensions`: Blacklist of file extensions

## Monitoring

### Prometheus Metrics

CloudAWSync exposes comprehensive metrics at `/metrics` endpoint (default port 9090):

- **Bandwidth**: Upload/download bytes
- **File Operations**: Counts and durations by operation type
- **System Resources**: CPU, memory, disk usage
- **Sync Statistics**: Files processed, errors, last sync time
- **Performance**: Active goroutines, queue sizes

### Logging

Structured logging with configurable levels and outputs:
- **Levels**: debug, info, warn, error
- **Formats**: json, text
- **Outputs**: file, stdout
- **Rotation**: Configurable log rotation

## Architecture

### Components

1. **Cloud Provider Interface**: Abstraction for cloud storage services
2. **File Watcher**: Real-time file system monitoring with batching
3. **Sync Engine**: Core synchronization logic with worker pools
4. **Metrics Collector**: Prometheus-based metrics collection
5. **Configuration Manager**: YAML-based configuration with validation
6. **Service Manager**: Main service orchestration

### Data Flow

```
File System Events → File Watcher → Sync Engine → Cloud Provider
                                        ↓
                                  Metrics Collector
```

### Concurrency Model

- **Worker Pools**: Separate pools for uploads and downloads
- **Event Batching**: Reduces redundant operations
- **Rate Limiting**: Configurable concurrency limits
- **Graceful Shutdown**: Proper resource cleanup

## Security Considerations

### File System Security
- Path validation prevents directory traversal
- Configurable file filters
- Permission preservation
- Atomic file operations

### Cloud Security
- Server-side encryption support
- Secure credential handling
- TLS for all communications
- IAM role support

### Operational Security
- Minimal required permissions
- Secure defaults
- Audit logging
- Error handling without information leakage

## Development

### Building from Source

```bash
# Clone repository
git clone https://github.com/aaronlmathis/CloudAWSync.git
cd CloudAWSync

# Install dependencies
go mod tidy

# Build
go build -o cloudawsync

# Run tests
go test ./...

# Build for production
go build -ldflags="-s -w" -o cloudawsync
```

### Project Structure

```
CloudAWSync/
├── main.go                     # Main application entry point
├── go.mod                      # Go module definition
├── internal/
│   ├── config/                 # Configuration management
│   ├── interfaces/             # Core interfaces
│   ├── providers/              # Cloud provider implementations
│   │   └── s3.go              # AWS S3 provider
│   ├── watcher/                # File system watching
│   ├── engine/                 # Sync engine
│   ├── metrics/                # Metrics collection
│   ├── service/                # Main service
│   └── utils/                  # Utility functions
└── README.md
```

### Adding New Cloud Providers

1. Implement the `CloudProvider` interface in `internal/interfaces/interfaces.go`
2. Add provider-specific configuration
3. Update service factory methods
4. Add tests

## Troubleshooting

### Common Issues

1. **Permission Denied**:
   - Check file permissions on sync directories
   - Verify AWS IAM permissions
   - Check systemd service user permissions

2. **High Memory Usage**:
   - Reduce concurrent worker counts
   - Decrease chunk sizes
   - Enable log rotation

3. **Sync Failures**:
   - Check network connectivity
   - Verify AWS credentials
   - Review file filters
   - Check disk space

### Debug Mode

Enable debug logging for detailed information:
```bash
./cloudawsync -log-level=debug -daemon=false
```

### Health Checks

Monitor service health using:
- Systemd status: `systemctl status cloudawsync`
- Logs: `journalctl -u cloudawsync -f`
- Metrics: `curl http://localhost:9090/metrics`

## Performance Optimization

### Tuning Guidelines

1. **Concurrent Workers**: Start with 5, adjust based on system resources
2. **Chunk Size**: 5MB works well for most use cases
3. **Batch Delay**: 2-5 seconds for event batching
4. **Bandwidth Limiting**: Set if network capacity is limited

### Resource Requirements

- **CPU**: 1-2 cores recommended
- **Memory**: 256MB minimum, 512MB recommended
- **Disk**: Minimal (logs and temporary files)
- **Network**: Varies based on sync volume

## License

This project is licensed under the GNU General Public License v3.0 or later. See the [LICENSE](LICENSE) file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## Support

- **Issues**: GitHub Issues
- **Documentation**: This README and inline code comments
- **Security**: Report security issues privately

## Roadmap

- [ ] Additional cloud providers (Google Cloud, Azure)
- [ ] Bidirectional sync support
- [ ] Conflict resolution strategies
- [ ] Web-based configuration interface
- [ ] Advanced scheduling options
- [ ] Compression support
- [ ] Deduplication
- [ ] Bandwidth scheduling
