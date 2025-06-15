# GoFileSync

A cross-platform Go application that watches a local folder and syncs changes to a remote SFTP server. Features interactive setup, secure config, service install for Windows/macOS/Linux, debug output, versioned builds, and robust CI/CD with GitHub Actions.

## Features
- Watches a local directory for file changes (fsnotify)
- Syncs changes to a remote SFTP server (pkg/sftp)
- Interactive setup and config
- Secure config storage (optionally using OS keyring)
- Service install for Windows/macOS/Linux
- Debug output and versioned builds
- Dev/release build distinction
- Automated GitHub Actions workflow

## Getting Started
- Build: `go build -o gofilesync .`
- Run: `./gofilesync`

## Versioned Builds

To build with a specific version embedded:

```
go build -ldflags="-X 'main.version=v1.2.3'" -o gofilesync .
```

Or use the VS Code task `build:versioned` and enter your version string when prompted.

The version is shown in the CLI with:

```
gofilesync --version
```

If you build without specifying a version, it defaults to `dev`.

## Running as a Service

You can install, start, stop, and uninstall GoFileSync as a background service/daemon:

```
# Install as a service
gofilesync service install

# Start the service
gofilesync service start

# Stop the service
gofilesync service stop

# Uninstall the service
gofilesync service uninstall
```

The service will use your `.gofilesync.json` config file.

## Service Config Location

When you install GoFileSync as a service, the config file is copied to a system-wide location:

- **Windows:** `%ProgramData%\gofilesync\gofilesync.json`
- **Linux:** `/etc/gofilesync/gofilesync.json`
- **macOS:** `/usr/local/etc/gofilesync/gofilesync.json`

The service always reads its config from this location. The CLI (`sync`, `setup`) uses `.gofilesync.json` in the current directory by default.

If you need to update the service config, edit the system config file or re-run `gofilesync service install` after updating your local config.

## License
MIT
