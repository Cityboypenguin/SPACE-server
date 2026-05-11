package logger

import (
	"os"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

func init() {
	level := zerolog.InfoLevel
	if os.Getenv("APP_ENV") != "production" {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
		Log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger().Level(level)
	} else {
		Log = zerolog.New(os.Stdout).With().Timestamp().Logger().Level(level)
	}
}
