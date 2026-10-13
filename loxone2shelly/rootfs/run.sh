#!/usr/bin/with-contenv bashio
# shellcheck shell=bash
set -e

# /config is the add-on's private config directory (map: addon_config).
CONFIG_DIR="/config"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"

mkdir -p "${CONFIG_DIR}"

LOG_LEVEL=$(bashio::config 'log_level' 'info')
API_TOKEN=$(bashio::config 'api_token' '')

# Migrate from the old location used by earlier versions.
if [ ! -f "${CONFIG_FILE}" ] && [ -f "/config/loxone2shelly/config.yaml" ]; then
    bashio::log.info "Migrating configuration from /config/loxone2shelly/config.yaml"
    mv /config/loxone2shelly/config.yaml "${CONFIG_FILE}"
fi

if [ ! -f "${CONFIG_FILE}" ]; then
    bashio::log.info "Creating initial configuration..."
    cat > "${CONFIG_FILE}" << EOF
server:
  port: 8080
  host: "0.0.0.0"

shelly:
  poll_interval: 10
  devices: []

loxone:
  miniservers: []

mqtt:
  enabled: false
  broker: "tcp://core-mosquitto:1883"
  client_id: "loxone2shelly"
  topic_prefix: "shellies"

logging:
  level: "${LOG_LEVEL}"
  format: "json"

mappings: []
EOF
    chmod 600 "${CONFIG_FILE}"
fi

# Keep the log level in sync with the add-on option (only the logging section).
if [ -n "${LOG_LEVEL}" ]; then
    sed -i -E '/^logging:/,/^[^ ]/ s/^(  level: ).*/\1"'"${LOG_LEVEL}"'"/' "${CONFIG_FILE}" 2>/dev/null || true
fi

bashio::log.info "Starting Loxone2Shelly Gateway..."
bashio::log.info "Config: ${CONFIG_FILE}"
bashio::log.info "Log Level: ${LOG_LEVEL}"
if [ -n "${API_TOKEN}" ]; then
    export L2S_API_TOKEN="${API_TOKEN}"
    bashio::log.info "API token: set via add-on option"
fi

exec /app/gateway -config "${CONFIG_FILE}"
