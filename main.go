package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/sftp"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
)

var version = "dev"

func main() {
	app := &cli.App{
		Name:    "gofilesync",
		Usage:   "Sync a local folder to an SFTP server (cross-platform)",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:  "setup",
				Usage: "Interactive setup and config",
				Action: func(c *cli.Context) error {
					return interactiveSetup(".gofilesync.json")
				},
			},
			{
				Name:  "sync",
				Usage: "Start watching and syncing",
				Action: func(c *cli.Context) error {
					cfg, err := loadConfig(".gofilesync.json")
					if err != nil {
						fmt.Println("Failed to load config:", err)
						return err
					}
					err = InitLogger(cfg.LogLevel, cfg.LogFile)
					if err != nil {
						fmt.Println("Failed to initialize logger:", err)
						return err
					}
					defer CloseLogger()
					LogInfo("Starting folder watcher and SFTP sync...")
					err = watchAndSync(cfg)
					if err != nil {
						LogError("Sync error: %v", err)
					}
					return err
				},
			},
			{
				Name:  "service",
				Usage: "Manage GoFileSync as a background service (install, uninstall, start, stop, run)",
				Subcommands: []*cli.Command{
					{
						Name:  "install",
						Usage: "Install GoFileSync as a service",
						Action: func(c *cli.Context) error {
							return runServiceCommand("install", ".gofilesync.json")
						},
					},
					{
						Name:  "uninstall",
						Usage: "Uninstall the GoFileSync service",
						Action: func(c *cli.Context) error {
							return runServiceCommand("uninstall", ".gofilesync.json")
						},
					},
					{
						Name:  "start",
						Usage: "Start the GoFileSync service",
						Action: func(c *cli.Context) error {
							return runServiceCommand("start", ".gofilesync.json")
						},
					},
					{
						Name:  "stop",
						Usage: "Stop the GoFileSync service",
						Action: func(c *cli.Context) error {
							return runServiceCommand("stop", ".gofilesync.json")
						},
					},
					{
						Name:  "run",
						Usage: "Run GoFileSync as a service (internal use)",
						Action: func(c *cli.Context) error {
							return runServiceCommand("run", ".gofilesync.json")
						},
					},
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func sftpConnect(cfg *Config) (*sftp.Client, error) {
	sshConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(cfg.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For demo; use a real callback in production
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, err
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func uploadFile(sftpClient *sftp.Client, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()
	_, err = remoteFile.ReadFrom(f)
	return err
}

func deleteFile(sftpClient *sftp.Client, remotePath string) error {
	return sftpClient.Remove(remotePath)
}

func watchAndSync(cfg *Config) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	sftpClient, err := sftpConnect(cfg)
	if err != nil {
		return fmt.Errorf("SFTP connect failed: %w", err)
	}
	defer sftpClient.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				LogDebug("event: %s", event)
				remoteFile := cfg.RemotePath + "/" + filepath.Base(event.Name)
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					LogInfo("Uploading: %s -> %s", event.Name, remoteFile)
					uploadFile(sftpClient, event.Name, remoteFile)
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					LogInfo("Deleting remote: %s", remoteFile)
					deleteFile(sftpClient, remoteFile)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				LogError("watcher error: %v", err)
			}
		}
	}()

	err = watcher.Add(cfg.LocalPath)
	if err != nil {
		return err
	}
	LogInfo("Watching for changes in %s", cfg.LocalPath)

	// Wait for interrupt
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	done <- true
	return nil
}
