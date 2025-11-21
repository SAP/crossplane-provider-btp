package cli

import (
	"log/slog"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

func configureLogging() {
	level := log.InfoLevel
	if viper.GetBool("verbose") {
		level = log.DebugLevel
	}
	slog.SetDefault(slog.New(log.NewWithOptions(os.Stdout, log.Options{
		Level: level,
	})))
}
