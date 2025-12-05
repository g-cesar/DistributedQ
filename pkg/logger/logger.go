package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger is the global logger instance
var Log zerolog.Logger

func init() {
	// Default to JSON output for production
	Log = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger()

	// Pretty print for development if requested
	if os.Getenv("APP_ENV") != "production" {
		Log = Log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}
}

// GetLogger returns the global logger instance
func GetLogger() zerolog.Logger {
	return Log
}
