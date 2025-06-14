package main

import (
	"io"
	"log"
	"os"
	"strings"
)

// LogLevel type for log levels
// Supported: debug, info, error

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	ERROR
)

var (
	logLevel LogLevel = INFO
	logFile  *os.File
	logger   *log.Logger
)

func ParseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "error":
		return ERROR
	default:
		return INFO
	}
}

func InitLogger(level, filePath string) error {
	logLevel = ParseLogLevel(level)
	var output io.Writer = os.Stdout
	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		logFile = f
		output = io.MultiWriter(os.Stdout, f)
	}
	logger = log.New(output, "", log.LstdFlags|log.Lshortfile)
	return nil
}

func CloseLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

func LogDebug(format string, v ...interface{}) {
	if logLevel <= DEBUG {
		logger.Printf("[DEBUG] "+format, v...)
	}
}

func LogInfo(format string, v ...interface{}) {
	if logLevel <= INFO {
		logger.Printf("[INFO] "+format, v...)
	}
}

func LogError(format string, v ...interface{}) {
	if logLevel <= ERROR {
		logger.Printf("[ERROR] "+format, v...)
	}
}
