package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/api"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/logging"
	"github.com/sbeyeler/loxone2shelly/internal/loxone"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to configuration file")
	flag.Parse()

	// Set version
	config.SetVersion(version)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Setup logging
	setupLogging(cfg.Logging)

	log.Info().Str("version", version).Msg("Loxone2Shelly Gateway starting")

	// Create Shelly client
	shellyClient := shelly.NewClient()
	for _, dev := range cfg.Shelly.Devices {
		shellyClient.AddDevice(dev.ID, dev.Name, dev.IP, dev.Generation)
	}

	// Create mapping manager
	mappingManager := loxone.NewMappingManager()
	mappingManager.Load(cfg.Mappings)

	// Create UDP sender
	udpSender := loxone.NewUDPSender()
	udpSender.Configure(cfg.Loxone.Miniservers)

	// Create HTTP sender
	httpSender := loxone.NewHTTPSender()
	httpSender.Configure(cfg.Loxone.Miniservers)

	// Start Shelly polling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if shellyClient.IsConfigured() {
		go shellyClient.StartPolling(ctx, cfg.Shelly.PollInterval)
		log.Info().Int("devices", len(cfg.Shelly.Devices)).Int("interval", cfg.Shelly.PollInterval).Msg("Shelly polling started")
	} else {
		log.Warn().Msg("No Shelly devices configured. Add devices via config file or API.")
	}

	// Create and start API server
	server := api.NewServer(shellyClient, mappingManager, udpSender, httpSender)

	// Start server in goroutine
	go func() {
		if err := server.Start(cfg.Server.Host, cfg.Server.Port); err != nil {
			log.Fatal().Err(err).Msg("API server failed")
		}
	}()

	log.Info().Int("port", cfg.Server.Port).Msg("Loxone2Shelly Gateway ready")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop polling
	cancel()

	// Shutdown API server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	// Close Shelly client
	shellyClient.Close()

	// Close senders
	udpSender.Close()

	log.Info().Msg("Loxone2Shelly Gateway stopped")
}

func setupLogging(cfg config.LoggingConfig) {
	// Set log level
	switch cfg.Level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Setup multi-writer: stdout + log buffer
	multiWriter := zerolog.MultiLevelWriter(os.Stdout, logging.Buffer)

	if cfg.Format == "console" {
		consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
		multiWriter = zerolog.MultiLevelWriter(consoleWriter, logging.Buffer)
	}

	log.Logger = zerolog.New(multiWriter).With().Timestamp().Logger()
}
