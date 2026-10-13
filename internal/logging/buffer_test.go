package logging

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestBufferParsesZerologOutput(t *testing.T) {
	b := NewLogBuffer(10)
	logger := zerolog.New(b).With().Timestamp().Logger()
	logger.Info().Str("device", "dev1").Msg("hello world")
	logger.Warn().Msg("careful")
	logger.Debug().Msg("noise")

	entries := b.GetEntries("", "", 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Message != "noise" || entries[2].Message != "hello world" {
		t.Fatalf("newest first expected: %+v", entries)
	}
	if entries[2].Level != "INFO" || entries[2].Fields["device"] != "dev1" || entries[2].Timestamp.IsZero() {
		t.Fatalf("entry wrong: %+v", entries[2])
	}
	if entries[2].Source != "SYSTEM" {
		t.Fatalf("source wrong: %q", entries[2].Source)
	}
}

func TestBufferFilters(t *testing.T) {
	b := NewLogBuffer(10)
	logger := zerolog.New(b)
	logger.Info().Str("id", "alpha").Msg("first")
	logger.Error().Msg("boom")
	logger.Info().Msg("second")

	if got := b.GetEntries("error", "", 0); len(got) != 1 || got[0].Message != "boom" {
		t.Fatalf("level filter: %+v", got)
	}
	if got := b.GetEntries("", "FIRST", 0); len(got) != 1 {
		t.Fatalf("message search should be case-insensitive: %+v", got)
	}
	if got := b.GetEntries("", "alpha", 0); len(got) != 1 || got[0].Message != "first" {
		t.Fatalf("field search: %+v", got)
	}
	if got := b.GetEntries("", "nothing", 0); len(got) != 0 {
		t.Fatalf("no match expected: %+v", got)
	}
	if got := b.GetEntries("", "", 2); len(got) != 2 || got[0].Message != "second" {
		t.Fatalf("limit: %+v", got)
	}
	if got := b.GetEntries("", "", 100); len(got) != 3 {
		t.Fatalf("limit above count: %+v", got)
	}
}

func TestBufferRingOverflowAndGarbage(t *testing.T) {
	b := NewLogBuffer(3)
	logger := zerolog.New(b)
	for i := 0; i < 5; i++ {
		logger.Info().Int("i", i).Msg("m")
	}
	entries := b.GetEntries("", "", 0)
	if len(entries) != 3 {
		t.Fatalf("ring must cap at 3, got %d", len(entries))
	}
	if entries[0].Fields["i"] != float64(4) || entries[2].Fields["i"] != float64(2) {
		t.Fatalf("ring kept wrong entries: %+v", entries)
	}

	n, err := b.Write([]byte("plain text line\n"))
	if err != nil || n != len("plain text line\n") {
		t.Fatalf("Write garbage: %d %v", n, err)
	}
	if len(b.GetEntries("", "", 0)) != 3 {
		t.Fatal("garbage must not create entries")
	}

	before := time.Now().Add(-time.Second)
	_, _ = b.Write([]byte(`{"level":"info","message":"no time"}`))
	e := b.GetEntries("", "no time", 1)
	if len(e) != 1 || e[0].Timestamp.Before(before) {
		t.Fatalf("fallback timestamp wrong: %+v", e)
	}
	_, _ = b.Write([]byte(`{"level":"info","message":"rfc3339","time":"2026-01-02T03:04:05Z"}`))
	e = b.GetEntries("", "rfc3339", 1)
	if len(e) != 1 || e[0].Timestamp.Year() != 2026 {
		t.Fatalf("RFC3339 timestamp not parsed: %+v", e)
	}
}

func TestGlobalBufferExists(t *testing.T) {
	if Buffer == nil || Buffer.size != 1000 {
		t.Fatal("global buffer must hold 1000 entries")
	}
}
