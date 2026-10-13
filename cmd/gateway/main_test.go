package main

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/logging"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]zerolog.Level{
		"debug":   zerolog.DebugLevel,
		"info":    zerolog.InfoLevel,
		"warn":    zerolog.WarnLevel,
		"error":   zerolog.ErrorLevel,
		"":        zerolog.InfoLevel,
		"verbose": zerolog.InfoLevel,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSetupLoggingFeedsBuffer(t *testing.T) {
	for _, format := range []string{"json", "console"} {
		setupLogging(config.LoggingConfig{Level: "debug", Format: format})
		if zerolog.GlobalLevel() != zerolog.DebugLevel {
			t.Fatalf("%s: global level not applied", format)
		}
		marker := "setup-logging-test-" + format
		zerologLogger().Info().Msg(marker)
		if got := logging.Buffer.GetEntries("", marker, 1); len(got) != 1 {
			t.Fatalf("%s: log line did not reach the ring buffer", format)
		}
	}

	setLogLevel("warn")
	if zerolog.GlobalLevel() != zerolog.WarnLevel {
		t.Fatal("setLogLevel not applied")
	}
	setLogLevel("info")
}

// zerologLogger returns the global logger configured by setupLogging.
func zerologLogger() *zerolog.Logger {
	return &log.Logger
}
