package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// logger.go centralizes structured logger setup and log file rotation.

var (
	once      sync.Once
	logPath   string
	logLevel  string = "info"
	loggerMap        = make(map[string]zerolog.Logger)
	mu        sync.RWMutex
)

// SetLogPath sets the directory for log files
func SetLogPath(path string) {
	logPath = path
}

// SetLogLevel sets the global log level
func SetLogLevel(level string) {
	logLevel = strings.ToLower(level)
}

// GetLogLevel returns the current log level as a string.
func GetLogLevel() string {
	return logLevel
}

// IsDebugEnabled reports whether debug-level logs are enabled.
func IsDebugEnabled() bool {
	return logLevel == "debug" || logLevel == "trace"
}

// IsInfoEnabled reports whether info-level logs are enabled.
func IsInfoEnabled() bool {
	switch logLevel {
	case "debug", "trace", "info":
		return true
	default:
		return false
	}
}

// IsTTY reports whether stdout is attached to a terminal.
func IsTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// GetLogPath returns the full path to the log file
func GetLogPath() string {
	if logPath == "" {
		logPath = "."
	}
	logsDir := filepath.Join(logPath, "logs")

	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logsDir, 0755); err != nil {
			fmt.Printf("Failed to create logs directory: %v\n", err)
			return filepath.Join(os.TempDir(), "robofuse.log")
		}
	}

	return filepath.Join(logsDir, "robofuse.log")
}

// New creates a new logger with the given prefix
func New(prefix string) zerolog.Logger {
	mu.RLock()
	if existing, ok := loggerMap[prefix]; ok {
		mu.RUnlock()
		return existing
	}
	mu.RUnlock()

	rotatingLogFile := &lumberjack.Logger{
		Filename: GetLogPath(),
		MaxSize:  10,
		MaxAge:   15,
		Compress: true,
	}

	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "15:04:05",
		NoColor:    false,
		FormatLevel: func(i interface{}) string {
			level := strings.ToUpper(fmt.Sprintf("%s", i))
			switch level {
			case "DEBUG":
				return "[DBG]"
			case "INFO":
				return "[INF]"
			case "WARN":
				return "[WRN]"
			case "ERROR":
				return "[ERR]"
			case "FATAL":
				return "[FTL]"
			default:
				return fmt.Sprintf("[%s]", level[:3])
			}
		},
		FormatMessage: func(i interface{}) string {
			return fmt.Sprintf("%v", i)
		},
	}

	fileWriter := zerolog.ConsoleWriter{
		Out:        rotatingLogFile,
		TimeFormat: "2006-01-02 15:04:05",
		NoColor:    true,
		FormatLevel: func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		},
		FormatMessage: func(i interface{}) string {
			return fmt.Sprintf("%v", i)
		},
	}

	multi := zerolog.MultiLevelWriter(consoleWriter, fileWriter)

	logger := zerolog.New(multi).
		With().
		Timestamp().
		Logger().
		Level(zerolog.InfoLevel)

	// Set the log level
	switch logLevel {
	case "debug":
		logger = logger.Level(zerolog.DebugLevel)
	case "info":
		logger = logger.Level(zerolog.InfoLevel)
	case "warn":
		logger = logger.Level(zerolog.WarnLevel)
	case "error":
		logger = logger.Level(zerolog.ErrorLevel)
	case "trace":
		logger = logger.Level(zerolog.TraceLevel)
	}

	mu.Lock()
	loggerMap[prefix] = logger
	mu.Unlock()

	return logger
}

// Default returns the default logger
func Default() zerolog.Logger {
	return New("robofuse")
}
