/*
SPDX-License-Identifier: GPL-3.0-or-later

Copyright (C) 2025 Aaron Mathis aaron.mathis@gmail.com

This file is part of CloudAWSync.

CloudAWSync is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

CloudAWSync is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with CloudAWSync. If not, see https://www.gnu.org/licenses/.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"CloudAWSync/internal/config"
	"CloudAWSync/internal/interfaces"
	"CloudAWSync/internal/service"
	"CloudAWSync/internal/utils"

	"go.uber.org/zap"
)

const (
	version = "1.0.0"
	appName = "CloudAWSync"
)

var (
	configPath     = flag.String("config", "", "Path to configuration file")
	showVersion    = flag.Bool("version", false, "Show version information")
	showHelp       = flag.Bool("help", false, "Show help information")
	daemon         = flag.Bool("daemon", true, "Run as daemon (default: true)")
	logLevel       = flag.String("log-level", "", "Override log level (debug, info, warn, error)")
	generateConfig = flag.Bool("generate-config", false, "Generate sample configuration file")
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s version %s\n", appName, version)
		os.Exit(0)
	}

	if *showHelp {
		showUsage()
		os.Exit(0)
	}

	if *generateConfig {
		if err := generateSampleConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Sample configuration generated successfully")
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Override log level if specified
	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}

	// Initialize logger
	logger, err := utils.InitLogger(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting CloudAWSync",
		zap.String("version", version),
		zap.String("config_path", getConfigPath(*configPath)))

	// Create and start service
	svc, err := service.NewService(cfg)
	if err != nil {
		logger.Fatal("Failed to create service", zap.Error(err))
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	// Start signal handler
	go func() {
		sig := <-sigChan
		logger.Info("Received signal, shutting down gracefully",
			zap.String("signal", sig.String()))

		// Stop the service
		if err := svc.Stop(); err != nil {
			logger.Error("Error during shutdown", zap.Error(err))
		}
		os.Exit(0)
	}()

	// Start service
	if err := svc.Start(); err != nil {
		logger.Fatal("Failed to start service", zap.Error(err))
	}

	// Run as daemon
	if *daemon {
		logger.Info("Running as daemon")
		svc.Wait()
	} else {
		// For testing/development, run for a short time
		logger.Info("Running in non-daemon mode for testing")
		time.Sleep(30 * time.Second)
	}

	// Graceful shutdown
	logger.Info("Shutting down service")
	if err := svc.Stop(); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
	}

	logger.Info("CloudAWSync stopped")
}

func showUsage() {
	fmt.Printf(`%s - Cloud File Synchronization Agent

Usage: %s [options]

Options:
  -config string
        Path to configuration file (default: searches standard locations)
  -daemon
        Run as daemon (default: true)
  -generate-config
        Generate sample configuration file
  -help
        Show this help message
  -log-level string
        Override log level (debug, info, warn, error)
  -version
        Show version information

Configuration File Locations (searched in order):
  1. Path specified by -config flag
  2. $XDG_CONFIG_HOME/cloudawsync/config.yaml
  3. $HOME/.config/cloudawsync/config.yaml
  4. /etc/cloudawsync/config.yaml

Environment Variables:
  AWS_ACCESS_KEY_ID     - AWS access key ID
  AWS_SECRET_ACCESS_KEY - AWS secret access key
  AWS_SESSION_TOKEN     - AWS session token (optional)
  AWS_REGION           - AWS region (default: us-east-1)

Examples:
  # Run with default configuration
  %s

  # Run with custom config file
  %s -config /path/to/config.yaml

  # Generate sample configuration
  %s -generate-config

  # Run in foreground with debug logging
  %s -daemon=false -log-level=debug

SystemD Service:
  To run as a systemd service, copy the generated service file to
  /etc/systemd/system/ and enable it:
  
  sudo systemctl enable cloudawsync
  sudo systemctl start cloudawsync

`, appName, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func generateSampleConfig() error {
	cfg := config.DefaultConfig()

	// Add sample directories
	cfg.Directories = []interfaces.SyncDirectory{
		{
			LocalPath:  "/home/user/Documents",
			RemotePath: "documents",
			SyncMode:   interfaces.SyncModeRealtime,
			Schedule:   "",
			Recursive:  true,
			Filters:    []string{"*.tmp", "*.lock", ".DS_Store"},
			Enabled:    true,
		},
		{
			LocalPath:  "/home/user/Pictures",
			RemotePath: "pictures",
			SyncMode:   interfaces.SyncModeScheduled,
			Schedule:   "0 2 * * *", // Daily at 2 AM
			Recursive:  true,
			Filters:    []string{"*.tmp", "Thumbs.db"},
			Enabled:    false, // Disabled by default
		},
	}

	configPath := "cloudawsync-config.yaml"
	if err := cfg.SaveConfig(configPath); err != nil {
		return err
	}

	fmt.Printf("Sample configuration saved to: %s\n", configPath)
	fmt.Println("\nIMPORTANT: Edit the configuration file to:")
	fmt.Println("1. Set your AWS credentials and S3 bucket")
	fmt.Println("2. Configure your directories to sync")
	fmt.Println("3. Adjust security and performance settings")
	fmt.Println("4. Enable directories you want to sync")

	return nil
}

func getConfigPath(providedPath string) string {
	if providedPath != "" {
		return providedPath
	}

	// Try standard locations
	locations := []string{
		os.Getenv("XDG_CONFIG_HOME") + "/cloudawsync/config.yaml",
		os.Getenv("HOME") + "/.config/cloudawsync/config.yaml",
		"/etc/cloudawsync/config.yaml",
	}

	for _, location := range locations {
		if location != "/cloudawsync/config.yaml" { // Skip if env var is empty
			if _, err := os.Stat(location); err == nil {
				return location
			}
		}
	}

	return "default configuration"
}

// generateSystemDService generates a systemd service file
func generateSystemDService(cfg *config.Config) error {
	serviceContent := fmt.Sprintf(`[Unit]
Description=CloudAWSync - Cloud File Synchronization Agent
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
WorkingDirectory=%s
ExecStart=%s -daemon=true
Restart=%s
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=cloudawsync

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=%s

# Resource limits
MemoryMax=512M
CPUQuota=50%%

[Install]
WantedBy=multi-user.target
`,
		cfg.SystemD.User,
		cfg.SystemD.Group,
		cfg.SystemD.WorkingDir,
		filepath.Join(cfg.SystemD.WorkingDir, "cloudawsync"),
		cfg.SystemD.RestartPolicy,
		cfg.SystemD.WorkingDir,
	)

	serviceFile := "cloudawsync.service"
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return err
	}

	fmt.Printf("SystemD service file generated: %s\n", serviceFile)
	fmt.Println("To install:")
	fmt.Printf("  sudo cp %s /etc/systemd/system/\n", serviceFile)
	fmt.Println("  sudo systemctl daemon-reload")
	fmt.Println("  sudo systemctl enable cloudawsync")
	fmt.Println("  sudo systemctl start cloudawsync")

	return nil
}
