package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/pkg/sftp"
	"github.com/rivo/tview"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/ssh"
	"gopkg.in/natefinch/lumberjack.v2"
)

var version = "dev"

// --- Config ---
type Config struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	RemotePath string `json:"remote_path"`
	LocalPath  string `json:"local_path"`
	LogFile    string `json:"log_file,omitempty"`
	Password   string `json:"password,omitempty"`
}

func loadConfig(configPath string) (*Config, error) {
	customPrint(fmt.Sprintf("Attempting to load config from: %s", configPath), DEBUG, false)
	data, err := os.ReadFile(configPath)
	if err != nil {
		customPrint(fmt.Sprintf("Error reading config file: %v", err), WARN, false)
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		customPrint(fmt.Sprintf("Error unmarshalling config JSON: %v", err), WARN, false)
		return nil, err
	}
	customPrint(fmt.Sprintf("Config loaded: %+v", cfg), DEBUG, false)
	return &cfg, nil
}

// --- Logging ---
type LogLevel int

const (
	WARN LogLevel = iota
	INFO
	DEBUG
	TRACE
)

var (
	logLevel    LogLevel = INFO
	logFile     *os.File
	logger      *log.Logger
	zapLogger   *zap.Logger
	logFilePath string
)

func InitLogger(debug bool, filePath string, disableConsole bool) error {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:     "time",
		LevelKey:    "level",
		MessageKey:  "msg",
		EncodeTime:  zapcore.ISO8601TimeEncoder,  // ISO8601 for timestamps
		EncodeLevel: zapcore.CapitalLevelEncoder, // Capitalized levels like [DEBUG]
		LineEnding:  " - ",                       // Separator between fields
	}

	var core zapcore.Core
	if filePath != "" {
		fileSyncer := zapcore.AddSync(&lumberjack.Logger{
			Filename:   filePath,
			MaxSize:    10, // megabytes
			MaxBackups: 3,
			MaxAge:     28, // days
		})
		if disableConsole {
			core = zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), fileSyncer, zapcore.DebugLevel)
		} else {
			consoleSyncer := zapcore.AddSync(os.Stdout)
			core = zapcore.NewTee(
				zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleSyncer, zapcore.DebugLevel),
				zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), fileSyncer, zapcore.DebugLevel),
			)
		}
	} else {
		consoleSyncer := zapcore.AddSync(os.Stdout)
		core = zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleSyncer, zapcore.DebugLevel)
	}

	zapLogger = zap.New(core)
	return nil
}

func CloseLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

func LogDebug(format string, v ...interface{}) {
	if logLevel == DEBUG {
		logger.Printf("[DEBUG] "+format, v...)
	}
}

func LogInfo(format string, v ...interface{}) {
	logger.Printf("[INFO] "+format, v...)
}

func LogError(format string, v ...interface{}) {
	logger.Printf("[ERROR] "+format, v...)
}

func setLogLevelFromArgs(logLevelArg string) {
	switch strings.ToLower(logLevelArg) {
	case "warn":
		logLevel = WARN
	case "info":
		logLevel = INFO
	case "debug":
		logLevel = DEBUG
	case "trace":
		logLevel = TRACE
	default:
		logLevel = INFO
	}
}

func customPrint(message string, level LogLevel, skipConsole bool) {
	var core zapcore.Core

	if skipConsole {
		// Log to file only if skipConsole is true and logfile is provided
		if logFilePath != "" {
			encoderConfig := zapcore.EncoderConfig{
				TimeKey:     "time",
				LevelKey:    "level",
				MessageKey:  "msg",
				EncodeTime:  zapcore.ISO8601TimeEncoder,  // ISO8601 for timestamps
				EncodeLevel: zapcore.CapitalLevelEncoder, // Capitalized levels like [DEBUG]
				LineEnding:  " - ",                       // Separator between fields
			}
			fileSyncer := zapcore.AddSync(&lumberjack.Logger{
				Filename:   logFilePath,
				MaxSize:    10, // megabytes
				MaxBackups: 3,
				MaxAge:     28, // days
			})
			core = zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), fileSyncer, zapcore.DebugLevel)
		} else {
			return // Do nothing if no logfile is provided
		}
	} else {
		// Log to both console and file
		consoleSyncer := zapcore.AddSync(os.Stdout)
		fileSyncer := zapcore.AddSync(&lumberjack.Logger{
			Filename:   logFilePath,
			MaxSize:    10, // megabytes
			MaxBackups: 3,
			MaxAge:     28, // days
		})
		core = zapcore.NewTee(
			zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()), consoleSyncer, zapcore.DebugLevel),
			zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), fileSyncer, zapcore.DebugLevel),
		)
	}

	logger := zap.New(core)
	defer logger.Sync()

	switch level {
	case WARN:
		logger.Warn(message)
	case INFO:
		logger.Info(message)
	case DEBUG:
		logger.Debug(message)
	case TRACE:
		logger.Debug(message) // Zap does not have TRACE, using DEBUG
	}
}

// --- tview Setup Wizard ---
func runTUISetup(configPath string) error {
	// Temporarily disable console logging for TUI
	if err := InitLogger(logLevel >= DEBUG, "", true); err != nil {
		return fmt.Errorf("failed to reconfigure logger for TUI: %w", err)
	}

	defer InitLogger(logLevel >= DEBUG, "", false) // Restore console logging after TUI

	customPrint("Entering runTUISetup, launching tview form...", DEBUG, true)
	app := tview.NewApplication()
	form := tview.NewForm().SetHorizontal(false)

	// Add logging mode indication
	logMode := "Normal Logging"
	switch logLevel {
	case WARN:
		logMode = "Logging Mode: WARN"
	case INFO:
		logMode = "Logging Mode: INFO"
	case DEBUG:
		logMode = "Logging Mode: DEBUG"
	case TRACE:
		logMode = "Logging Mode: TRACE"
	}
	form.AddTextView("", logMode, 40, 1, false, false)

	var host, port, username, password, remotePath, localPath string

	// Helper to update input fields from file browsers
	updateField := func(label, value string) {
		for i := 0; i < form.GetFormItemCount(); i++ {
			if form.GetFormItem(i).GetLabel() == label {
				if input, ok := form.GetFormItem(i).(*tview.InputField); ok {
					input.SetText(value)
				}
			}
		}
	}

	form.AddInputField("SFTP Host", "", 40, nil, func(text string) { host = text })
	form.AddInputField("SFTP Port", "22", 6, nil, func(text string) { port = text })
	form.AddInputField("SFTP Username", "", 20, nil, func(text string) { username = text })
	form.AddPasswordField("SFTP Password", "", 20, '*', func(text string) { password = text })
	form.AddInputField("Remote SFTP Path", "/", 40, nil, func(text string) { remotePath = text })
	form.AddButton("Browse Remote", func() {
		// Always fetch current values from form fields
		hostField := form.GetFormItemByLabel("SFTP Host").(*tview.InputField)
		portField := form.GetFormItemByLabel("SFTP Port").(*tview.InputField)
		userField := form.GetFormItemByLabel("SFTP Username").(*tview.InputField)
		passField := form.GetFormItemByLabel("SFTP Password").(*tview.InputField)
		host = hostField.GetText()
		port = portField.GetText()
		username = userField.GetText()
		password = passField.GetText()
		if host == "" || port == "" || username == "" || password == "" {
			modal := tview.NewModal().SetText("Please fill in SFTP Host, Port, Username, and Password first.").AddButtons([]string{"OK"})
			modal.SetDoneFunc(func(_ int, _ string) { app.SetRoot(form, true) })
			app.SetRoot(modal, true)
			return
		}
		p, err := atoi(port), error(nil)
		if p == 0 {
			modal := tview.NewModal().SetText("Invalid port number.").AddButtons([]string{"OK"})
			modal.SetDoneFunc(func(_ int, _ string) { app.SetRoot(form, true) })
			app.SetRoot(modal, true)
			return
		}
		customPrint("Connecting to SFTP for remote browse...", DEBUG, true)
		client, err := connectSFTP(host, p, username, password)
		if err != nil {
			modal := tview.NewModal().SetText("SFTP connection failed: " + err.Error()).AddButtons([]string{"OK"})
			modal.SetDoneFunc(func(_ int, _ string) { app.SetRoot(form, true) })
			app.SetRoot(modal, true)
			return
		}
		startDir := "/"
		browser := tview.NewTreeView()
		root := tview.NewTreeNode(startDir).SetColor(tview.Styles.PrimaryTextColor)
		browser.SetRoot(root).SetCurrentNode(root)
		addDirChildren := func(node *tview.TreeNode, path string) {
			node.ClearChildren()
			// Only add '.. (up)' if this is the root node
			if node == browser.GetRoot() {
				parent := filepath.Dir(path)
				if parent != path {
					parentNode := tview.NewTreeNode(".. (up)").SetReference(parent)
					parentNode.SetColor(tview.Styles.TertiaryTextColor)
					node.AddChild(parentNode)
				}
			}
			files, err := client.ReadDir(path)
			customPrint(fmt.Sprintf("Reading directory: %s", path), DEBUG, true)
			if err != nil {
				customPrint(fmt.Sprintf("Error reading directory %s: %v", path, err), WARN, true)
				return
			}
			if len(files) == 0 {
				customPrint(fmt.Sprintf("No files found in directory: %s", path), DEBUG, true)
			}
			for _, f := range files {
				if f.IsDir() {
					child := tview.NewTreeNode(f.Name()).SetReference(filepath.Join(path, f.Name()))
					child.SetColor(tview.Styles.SecondaryTextColor)
					node.AddChild(child)
				}
			}
		}
		addDirChildren(root, startDir)
		browser.SetSelectedFunc(func(node *tview.TreeNode) {
			ref := node.GetReference()
			if ref != nil {
				selected := ref.(string)
				if node.GetText() == ".. (up)" {
					newRoot := tview.NewTreeNode(selected).SetColor(tview.Styles.PrimaryTextColor)
					addDirChildren(newRoot, selected)
					browser.SetRoot(newRoot).SetCurrentNode(newRoot)
				} else {
					addDirChildren(node, selected)
					browser.SetCurrentNode(node)
				}
			}
		})
		// Add key handler for navigation and selection
		browser.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			customPrint(fmt.Sprintf("Remote browser key: %v (%v)", event.Key(), event.Name()), DEBUG, true)
			currentNode := browser.GetCurrentNode()
			if event.Key() == tcell.KeyEnter {
				if currentNode != nil {
					ref := currentNode.GetReference()
					if ref != nil && currentNode.GetText() != ".. (up)" {
						selected := ref.(string)
						customPrint(fmt.Sprintf("Remote browser: Enter pressed, selecting %s", selected), DEBUG, true)
						remotePath = selected
						updateField("Remote SFTP Path", selected)
						app.SetRoot(form, true)
						return nil
					}
				}
			} else if event.Key() == tcell.KeyLeft {
				if currentNode != nil {
					ref := currentNode.GetReference()
					if ref != nil {
						parent := filepath.Dir(ref.(string))
						if parent != ref.(string) {
							customPrint(fmt.Sprintf("Remote browser: Left arrow, going up to %s", parent), DEBUG, true)
							newRoot := tview.NewTreeNode(parent).SetColor(tview.Styles.PrimaryTextColor)
							// Show loading indicator
							loading := tview.NewTextView().SetText("Loading...").SetTextColor(tcell.ColorYellow)
							flex := tview.NewFlex().SetDirection(tview.FlexRow).
								AddItem(loading, 1, 0, false).
								AddItem(browser, 0, 1, true)
							app.SetRoot(flex, true)
							addDirChildren(newRoot, parent)
							browser.SetRoot(newRoot).SetCurrentNode(newRoot)
							app.SetRoot(flex, true)
						}
					}
				}
				return nil
			} else if event.Key() == tcell.KeyRight {
				if currentNode != nil {
					ref := currentNode.GetReference()
					if ref != nil && currentNode.GetText() != ".. (up)" {
						customPrint(fmt.Sprintf("Remote browser: Right arrow, expanding %s", ref.(string)), DEBUG, true)
						if len(currentNode.GetChildren()) == 0 {
							// Show loading indicator
							loading := tview.NewTextView().SetText("Loading...").SetTextColor(tcell.ColorYellow)
							flex := tview.NewFlex().SetDirection(tview.FlexRow).
								AddItem(loading, 1, 0, false).
								AddItem(browser, 0, 1, true)
							app.SetRoot(flex, true)
							addDirChildren(currentNode, ref.(string))
							app.SetRoot(flex, true)
						}
						browser.SetCurrentNode(currentNode)
					}
				}
				return nil
			} else if event.Key() == tcell.KeyEsc {
				app.SetRoot(form, true)
				return nil
			}
			return event
		})
		// Show navigation instructions
		instructions := tview.NewTextView().SetText("Navigate: ↑↓ arrows | → enter dir | ← up dir | Enter select | Esc/Cancel").SetTextColor(tcell.ColorYellow)
		flex := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(instructions, 1, 0, false).
			AddItem(browser, 0, 1, true)
		app.SetRoot(flex, true)
	})
	form.AddInputField("Local Directory", "", 40, nil, func(text string) { localPath = text })
	form.AddButton("Browse Local", func() {
		current := localPath
		if current == "" {
			current, _ = os.Getwd()
		}
		browser := tview.NewTreeView()
		root := tview.NewTreeNode(current).SetColor(tview.Styles.PrimaryTextColor)
		browser.SetRoot(root).SetCurrentNode(root)
		addDirChildren := func(node *tview.TreeNode, path string) {
			node.ClearChildren()
			// Only add '.. (up)' if this is the root node
			if node == browser.GetRoot() {
				parent := filepath.Dir(path)
				if parent != path {
					parentNode := tview.NewTreeNode(".. (up)").SetReference(parent)
					parentNode.SetColor(tview.Styles.TertiaryTextColor)
					node.AddChild(parentNode)
				}
			}
			files, err := os.ReadDir(path)
			if err != nil {
				return
			}
			for _, f := range files {
				if f.IsDir() {
					child := tview.NewTreeNode(f.Name()).SetReference(filepath.Join(path, f.Name()))
					child.SetColor(tview.Styles.SecondaryTextColor)
					node.AddChild(child)
				}
			}
		}
		addDirChildren(root, current)
		browser.SetSelectedFunc(func(node *tview.TreeNode) {
			ref := node.GetReference()
			if ref != nil {
				selected := ref.(string)
				if node.GetText() == ".. (up)" {
					// Go up a directory: reset root to parent
					newRoot := tview.NewTreeNode(selected).SetColor(tview.Styles.PrimaryTextColor)
					addDirChildren(newRoot, selected)
					browser.SetRoot(newRoot).SetCurrentNode(newRoot)
				} else {
					addDirChildren(node, selected)
					localPath = selected
					updateField("Local Directory", selected)
					app.SetRoot(form, true)
				}
			}
		})
		// Remove unused modal and add Esc key to cancel/return to form
		browser.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			currentNode := browser.GetCurrentNode()
			if event.Key() == tcell.KeyEnter {
				if currentNode != nil {
					ref := currentNode.GetReference()
					if ref != nil && currentNode.GetText() != ".. (up)" {
						selected := ref.(string)
						localPath = selected
						updateField("Local Directory", selected)
						app.SetRoot(form, true)
						return nil
					}
				}
			} else if event.Key() == tcell.KeyLeft {
				// Go up a directory
				if currentNode != nil {
					ref := currentNode.GetReference()
					if ref != nil {
						parent := filepath.Dir(ref.(string))
						if parent != ref.(string) {
							newRoot := tview.NewTreeNode(parent).SetColor(tview.Styles.PrimaryTextColor)
							addDirChildren(newRoot, parent)
							browser.SetRoot(newRoot).SetCurrentNode(newRoot)
						}
					}
				}
				return nil
			} else if event.Key() == tcell.KeyRight {
				// Try to go down a level (expand directory)
				if currentNode != nil {
					ref := currentNode.GetReference()
					if ref != nil && currentNode.GetText() != ".. (up)" {
						// Only add children if not already loaded
						if len(currentNode.GetChildren()) == 0 {
							addDirChildren(currentNode, ref.(string))
						}
						browser.SetCurrentNode(currentNode)
					}
				}
				return nil
			} else if event.Key() == tcell.KeyEsc {
				app.SetRoot(form, true)
				return nil
			}
			return event
		})
		// Show navigation instructions
		instructions := tview.NewTextView().SetText("Navigate: ↑↓ arrows | → enter dir | ← up dir | Enter select | Esc/Cancel").SetTextColor(tcell.ColorYellow)
		flex := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(instructions, 1, 0, false).
			AddItem(browser, 0, 1, true)
		app.SetRoot(flex, true)
	})
	form.AddButton("Save", func() {
		cfg := Config{
			Host:       host,
			Port:       atoi(port),
			Username:   username,
			Password:   password,
			RemotePath: remotePath,
			LocalPath:  localPath,
		}
		if logLevel <= DEBUG {
			fmt.Printf("[DEBUG] Saving config: %+v\n", cfg)
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			customPrint(fmt.Sprintf("[DEBUG] Error marshalling config to JSON: %v", err), WARN, true)
		}
		err = os.WriteFile(configPath, data, 0600)
		if err != nil {
			customPrint(fmt.Sprintf("[DEBUG] Error writing config file: %v", err), WARN, true)
		}
		app.Stop()
		customPrint(fmt.Sprintf("Config saved to %s", configPath), INFO, true)
	})
	form.AddButton("Cancel", func() {

		customPrint("[DEBUG] Setup cancelled by user.", DEBUG, true)
		app.Stop()
		customPrint("Setup cancelled.", INFO, false)
	})

	form.SetBorder(true).SetTitle("GoFileSync Setup").SetTitleAlign(tview.AlignLeft)
	if err := app.SetRoot(form, true).Run(); err != nil {
		if logLevel <= DEBUG {
			customPrint(fmt.Sprintf("[DEBUG] tview application error: %v", err), WARN, true)
		}
		return err
	}
	return nil
}

// --- Main ---
func main() {
	// Initialize logger as early as possible
	logFilePath = getDefaultLogFilePath("gofilesync", version, false)
	err := InitLogger(logLevel >= DEBUG, logFilePath, false)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	configPath := "config.json"

	// Define flags
	logLevelArg := flag.String("loglevel", "info", "Set log level (options: warn, info, debug, trace)")
	logfileFlag := flag.Bool("logfile", false, "Enable logging to a file (auto-named)")
	helpFlag := flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *helpFlag {
		displayHelp()
		return
	}

	// Set log level from arguments before any other operations
	setLogLevelFromArgs(*logLevelArg)

	isService := false
	if len(os.Args) > 1 && os.Args[1] == "service" {
		isService = true
	}
	if *logfileFlag {
		logFilePath = getDefaultLogFilePath("gofilesync", version, isService)
	}

	// Reinitialize logger with updated settings
	err = InitLogger(logLevel >= DEBUG, logFilePath, false)
	if err != nil {
		fmt.Printf("Failed to reinitialize logger: %v\n", err)
		os.Exit(1)
	}

	args := flag.Args()
	cmd := ""
	if len(args) < 1 {
		if _, err := os.Stat(configPath); err == nil {
			cmd = "start"
			customPrint("Config file found, proceeding to start mode.", DEBUG, false)
		} else {
			cmd = "setup"
			customPrint("Config file not found, entering setup mode.", DEBUG, false)
		}
	} else {
		cmd = args[0]
		customPrint(fmt.Sprintf("Command line argument detected: %s", cmd), DEBUG, false)
	}

	customPrint(fmt.Sprintf("Service mode: %v", isService), DEBUG, false)
	if *logfileFlag {
		customPrint(fmt.Sprintf("Auto log file path set to: %s", logFilePath), DEBUG, false)
	}

	switch cmd {
	case "setup":
		customPrint("Running setup wizard...", DEBUG, false)
		err := runTUISetup(configPath)
		if err != nil {
			customPrint(fmt.Sprintf("Setup failed: %v", err), WARN, false)
			os.Exit(1)
		}
	case "start":
		customPrint("Loading config and starting sync...", DEBUG, false)
		_, err := loadConfig(configPath)
		if err != nil {
			customPrint(fmt.Sprintf("Failed to load config: %v", err), WARN, false)
			os.Exit(1)
		}
		customPrint("Starting folder-to-SFTP sync (not implemented in this stub)", INFO, false)
	case "stop":
		customPrint("Stop command received.", DEBUG, false)
	case "version":
		customPrint("Version command received.", DEBUG, false)
		customPrint(fmt.Sprintf("gofilesync version: %s", version), INFO, false)
	default:
		customPrint(fmt.Sprintf("Unknown command: %s", cmd), WARN, false)
		displayHelp()
		os.Exit(1)
	}
}

func displayHelp() {
	helpText := `Usage: gofilesync [options] [command]

Options:
  --loglevel <level>   Set log level (options: warn, info, debug, trace). Default: info
  --logfile            Enable logging to a file (auto-named).
  --help               Show this help message.

Commands:
  setup                Launch the setup wizard.
  start                Start the folder-to-SFTP sync.
  stop                 Stop the running sync process.
  version              Display the application version.`
	zapLogger.Info(helpText) // Replacing fmt.Println to avoid TUI clobbering
}

func makeAutoLogFileName(app, version string) string {
	now := time.Now()
	return fmt.Sprintf("%s.%s.%02d-%02d-%04d.%02d%02d.log",
		app,
		version,
		now.Day(), now.Month(), now.Year(),
		now.Hour(), now.Minute(),
	)
}

func getDefaultLogFilePath(app, version string, isService bool) string {
	if isService {
		// On Windows, use ProgramData or a system log directory for services
		if runtime.GOOS == "windows" {
			return filepath.Join(os.Getenv("ProgramData"), app, app+".log")
		}
		// On Linux, use /var/log/appname.log
		return filepath.Join("/var/log", app, app+".log")
	}

	// Use the current working directory if not running as a service
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "." // Fallback to current directory
	}
	return filepath.Join(cwd, makeAutoLogFileName(app, version))
}

func connectSFTP(host string, port int, user, pass string) (*sftp.Client, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		customPrint(fmt.Sprintf("SSH dial error: %v", err), DEBUG, true)
		return nil, err
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		customPrint(fmt.Sprintf("SFTP client error: %v", err), DEBUG, true)
		return nil, err
	}
	customPrint("SFTP connection established.", DEBUG, true)
	return client, nil
}

func atoi(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

// Set a global logger with the custom format immediately
func init() {
	logFilePath = getDefaultLogFilePath("gofilesync", version, false)
	_ = InitLogger(logLevel >= DEBUG, logFilePath, false) // Ignore errors for early initialization
}
