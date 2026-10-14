package main

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/sbeyeler/loxone2shelly/internal/mqttclient"
	"github.com/sbeyeler/loxone2shelly/internal/shelly"
)

// version is injected at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	config.SetVersion(version)

	// Provisional logging until the configuration is known.
	setupLogging(config.LoggingConfig{Level: "info", Format: "json"})

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}
	setupLogging(cfg.Logging)

	log.Info().Str("version", version).Str("config", *configPath).Msg("Loxone2Shelly Gateway starting")
	if cfg.Server.APIToken == "" {
		log.Warn().Msg("No API token configured (server.api_token / L2S_API_TOKEN): the API and Web UI are unauthenticated")
	}
	if cfg.Server.WebhookToken == "" {
		log.Warn().Msg("No webhook token configured (server.webhook_token): /api/webhook accepts unauthenticated calls")
	}

	// Shelly client
	shellyClient := shelly.NewClient()
	for _, dev := range cfg.Shelly.Devices {
		shellyClient.AddDevice(shelly.DeviceSpec{
			ID: dev.ID, Name: dev.Name, IP: dev.IP, Generation: dev.Generation,
			Username: dev.Username, Password: dev.Password,
		})
	}

	// Mappings and Loxone senders
	mappingManager := loxone.NewMappingManager()
	mappingManager.Load(cfg.Mappings)

	udpSender := loxone.NewUDPSender()
	udpSender.Configure(cfg.Loxone.Miniservers)

	httpSender := loxone.NewHTTPSender()
	httpSender.Configure(cfg.Loxone.Miniservers)

	mqttSender := loxone.NewMQTTSender()
	mqttSender.Configure(cfg.MQTT, cfg.Loxone.Miniservers)

	dispatcher := loxone.NewDispatcher(mappingManager, udpSender, httpSender, mqttSender)

	// Shelly polling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go shellyClient.StartPolling(ctx, cfg.Shelly.PollInterval)
	if !shellyClient.IsConfigured() {
		log.Warn().Msg("No Shelly devices configured. Add devices via config file or Web UI.")
	}

	// MQTT subscriber (always created so it can be enabled at runtime)
	mqttSub := mqttclient.NewSubscriber(shellyClient)
	mqttSub.SetDevices(cfg.Shelly.Devices)
	if err := mqttSub.Start(cfg.MQTT); err != nil {
		log.Error().Err(err).Msg("Failed to start MQTT subscriber (continuing without MQTT)")
	}

	// API server
	server := api.NewServer(api.Deps{
		Shelly:      shellyClient,
		Mappings:    mappingManager,
		UDP:         udpSender,
		HTTP:        httpSender,
		MQTT:        mqttSender,
		Dispatcher:  dispatcher,
		MQTTSub:     mqttSub,
		SetLogLevel: setLogLevel,
	})

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start(cfg.Server.Host, cfg.Server.Port)
	}()

	log.Info().Int("port", cfg.Server.Port).Msg("Loxone2Shelly Gateway ready")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			log.Error().Err(err).Msg("API server failed")
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}
	mqttSub.Close()
	shellyClient.Close()
	udpSender.Close()
	httpSender.Close()
	mqttSender.Close()

	log.Info().Msg("Loxone2Shelly Gateway stopped")
}

func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

func setLogLevel(level string) {
	zerolog.SetGlobalLevel(parseLevel(level))
	log.Info().Str("level", level).Msg("Log level changed")
}

func setupLogging(cfg config.LoggingConfig) {
	zerolog.SetGlobalLevel(parseLevel(cfg.Level))

	var out zerolog.LevelWriter
	if cfg.Format == "console" {
		out = zerolog.MultiLevelWriter(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}, logging.Buffer)
	} else {
		out = zerolog.MultiLevelWriter(os.Stdout, logging.Buffer)
	}

	log.Logger = zerolog.New(out).With().Timestamp().Logger()
}
