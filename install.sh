#!/bin/bash

# CloudAWSync Installation and Setup Script
# This script demonstrates the complete setup process for CloudAWSync

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/opt/cloudawsync"
CONFIG_DIR="/etc/cloudawsync"
LOG_DIR="/var/log/cloudawsync"
SERVICE_USER="cloudawsync"
SERVICE_GROUP="cloudawsync"

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

check_dependencies() {
    log_info "Checking dependencies..."
    
    # Check if Go is installed (for building)
    if ! command -v go &> /dev/null; then
        log_warning "Go is not installed. Installing Go 1.21..."
        wget -qO- https://go.dev/dl/go1.21.6.linux-amd64.tar.gz | tar -C /usr/local -xz
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
        export PATH=$PATH:/usr/local/go/bin
    fi
    
    # Check if systemctl is available
    if ! command -v systemctl &> /dev/null; then
        log_error "systemctl is not available. This script requires systemd."
        exit 1
    fi
    
    log_success "Dependencies checked"
}

create_user() {
    log_info "Creating service user and group..."
    
    if ! getent group "$SERVICE_GROUP" > /dev/null 2>&1; then
        groupadd --system "$SERVICE_GROUP"
        log_success "Created group: $SERVICE_GROUP"
    fi
    
    if ! getent passwd "$SERVICE_USER" > /dev/null 2>&1; then
        useradd --system --gid "$SERVICE_GROUP" --home-dir "$INSTALL_DIR" \
                --shell /bin/false --comment "CloudAWSync service user" "$SERVICE_USER"
        log_success "Created user: $SERVICE_USER"
    fi
}

create_directories() {
    log_info "Creating directories..."
    
    mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$LOG_DIR"
    chown "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR" "$LOG_DIR"
    chmod 755 "$INSTALL_DIR" "$CONFIG_DIR"
    chmod 750 "$LOG_DIR"
    
    log_success "Created directories"
}

build_application() {
    log_info "Building CloudAWSync..."
    
    # Get the current directory
    CURRENT_DIR=$(pwd)
    
    # Build the application
    go mod tidy
    go build -ldflags="-s -w" -o "$INSTALL_DIR/cloudawsync" .
    chown "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR/cloudawsync"
    chmod 755 "$INSTALL_DIR/cloudawsync"
    
    log_success "Built CloudAWSync binary"
}

install_config() {
    log_info "Installing configuration..."
    
    # Generate sample config
    "$INSTALL_DIR/cloudawsync" -generate-config
    mv cloudawsync-config.yaml "$CONFIG_DIR/config.yaml"
    chown root:root "$CONFIG_DIR/config.yaml"
    chmod 644 "$CONFIG_DIR/config.yaml"
    
    # Copy example config
    if [ -f "config.yaml.example" ]; then
        cp config.yaml.example "$CONFIG_DIR/config.yaml.example"
        chmod 644 "$CONFIG_DIR/config.yaml.example"
    fi
    
    log_success "Installed configuration files"
}

install_systemd_service() {
    log_info "Installing systemd service..."
    
    cat > /etc/systemd/system/cloudawsync.service << EOF
[Unit]
Description=CloudAWSync - Cloud File Synchronization Agent
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/cloudawsync -config=$CONFIG_DIR/config.yaml -daemon=true
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
ReadWritePaths=$LOG_DIR $CONFIG_DIR

# Resource limits
MemoryMax=512M
CPUQuota=50%

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    log_success "Installed systemd service"
}

setup_logrotate() {
    log_info "Setting up log rotation..."
    
    cat > /etc/logrotate.d/cloudawsync << EOF
$LOG_DIR/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 640 $SERVICE_USER $SERVICE_GROUP
    postrotate
        systemctl reload cloudawsync || true
    endscript
}
EOF

    log_success "Configured log rotation"
}

install_completion() {
    log_info "Installing bash completion..."
    
    cat > /etc/bash_completion.d/cloudawsync << 'EOF'
_cloudawsync() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="-config -daemon -generate-config -help -log-level -version"

    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi

    case "${prev}" in
        -config)
            COMPREPLY=( $(compgen -f ${cur}) )
            return 0
            ;;
        -log-level)
            COMPREPLY=( $(compgen -W "debug info warn error" -- ${cur}) )
            return 0
            ;;
        *)
            ;;
    esac
}

complete -F _cloudawsync cloudawsync
EOF

    log_success "Installed bash completion"
}

show_final_instructions() {
    log_success "CloudAWSync installation completed!"
    echo
    log_info "Next steps:"
    echo "1. Edit the configuration file: $CONFIG_DIR/config.yaml"
    echo "   - Set your AWS credentials and S3 bucket"
    echo "   - Configure directories to sync"
    echo "   - Adjust security and performance settings"
    echo
    echo "2. Enable and start the service:"
    echo "   systemctl enable cloudawsync"
    echo "   systemctl start cloudawsync"
    echo
    echo "3. Check service status:"
    echo "   systemctl status cloudawsync"
    echo "   journalctl -u cloudawsync -f"
    echo
    echo "4. View metrics (if enabled):"
    echo "   curl http://localhost:9090/metrics"
    echo
    log_info "Configuration file location: $CONFIG_DIR/config.yaml"
    log_info "Log files location: $LOG_DIR/"
    log_info "Service binary location: $INSTALL_DIR/cloudawsync"
    echo
    log_warning "Remember to configure AWS credentials before starting the service!"
}

show_usage() {
    echo "CloudAWSync Installation Script"
    echo
    echo "Usage: $0 [options]"
    echo
    echo "Options:"
    echo "  install     - Full installation (default)"
    echo "  uninstall   - Remove CloudAWSync"
    echo "  status      - Show service status"
    echo "  restart     - Restart service"
    echo "  logs        - Show service logs"
    echo "  help        - Show this help"
    echo
}

uninstall() {
    log_info "Uninstalling CloudAWSync..."
    
    # Stop and disable service
    systemctl stop cloudawsync || true
    systemctl disable cloudawsync || true
    
    # Remove files
    rm -f /etc/systemd/system/cloudawsync.service
    rm -f /etc/logrotate.d/cloudawsync
    rm -f /etc/bash_completion.d/cloudawsync
    rm -rf "$INSTALL_DIR"
    rm -rf "$LOG_DIR"
    
    # Remove user and group
    userdel "$SERVICE_USER" || true
    groupdel "$SERVICE_GROUP" || true
    
    systemctl daemon-reload
    
    log_success "CloudAWSync uninstalled"
    log_warning "Configuration files in $CONFIG_DIR were preserved"
}

show_status() {
    echo "=== CloudAWSync Service Status ==="
    systemctl status cloudawsync
    echo
    echo "=== Recent Logs ==="
    journalctl -u cloudawsync -n 20 --no-pager
}

show_logs() {
    journalctl -u cloudawsync -f
}

restart_service() {
    log_info "Restarting CloudAWSync service..."
    systemctl restart cloudawsync
    log_success "Service restarted"
}

# Main script logic
case "${1:-install}" in
    install)
        check_root
        check_dependencies
        create_user
        create_directories
        build_application
        install_config
        install_systemd_service
        setup_logrotate
        install_completion
        show_final_instructions
        ;;
    uninstall)
        check_root
        uninstall
        ;;
    status)
        show_status
        ;;
    restart)
        check_root
        restart_service
        ;;
    logs)
        show_logs
        ;;
    help|--help|-h)
        show_usage
        ;;
    *)
        log_error "Unknown option: $1"
        show_usage
        exit 1
        ;;
esac
