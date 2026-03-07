#!/usr/bin/with-contenv bashio

CONFIG_DIR="/config/loxone2shelly"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"

# Create config directory if not exists
mkdir -p "${CONFIG_DIR}"

# Read add-on options
LOG_LEVEL=$(bashio::config 'log_level' 'info')

# Create config file if it doesn't exist
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

logging:
  level: "${LOG_LEVEL}"
  format: "json"

mappings: []
EOF
fi

# Update log level from add-on options
if [ -n "${LOG_LEVEL}" ]; then
    sed -i "s/level: .*/level: \"${LOG_LEVEL}\"/" "${CONFIG_FILE}" 2>/dev/null || true
fi

bashio::log.info "Starting Loxone2Shelly Gateway..."
bashio::log.info "Config: ${CONFIG_FILE}"
bashio::log.info "Log Level: ${LOG_LEVEL}"

# Start the gateway
exec /app/gateway -config "${CONFIG_FILE}"
