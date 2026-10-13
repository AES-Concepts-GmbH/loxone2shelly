package api

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
)

//go:embed swagger-ui.html
var swaggerUIHTML string

// SwaggerUI serves the Swagger UI HTML page
func (h *Handlers) SwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(swaggerUIHTML)); err != nil {
		log.Debug().Err(err).Msg("Failed to write swagger UI")
	}
}

// SwaggerJSON serves the OpenAPI specification
func (h *Handlers) SwaggerJSON(w http.ResponseWriter, r *http.Request) {
	spec := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "Loxone2Shelly API",
			"description": "REST API for the Loxone2Shelly Gateway. Bridges Loxone Miniserver with Shelly smart home devices.",
			"version":     config.GetVersion(),
		},
		"servers": []map[string]interface{}{
			{"url": "/", "description": "Current server"},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{"type": "http", "scheme": "bearer"},
				"apiKey":     map[string]interface{}{"type": "apiKey", "in": "header", "name": "X-API-Key"},
				"queryToken": map[string]interface{}{"type": "apiKey", "in": "query", "name": "token"},
			},
		},
		"security": []map[string]interface{}{{"bearerAuth": []string{}}, {"apiKey": []string{}}, {"queryToken": []string{}}},
		"paths": map[string]interface{}{
			"/api/webhook": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Shelly action-URL webhook",
					"description": "Called by Shelly devices (Actions / I/O URL actions). Unauthenticated unless server.webhook_token is set.",
					"tags":        []string{"Webhook"},
					"security":    []map[string]interface{}{},
					"parameters": []map[string]interface{}{
						{"name": "device_id", "in": "query", "required": true, "schema": map[string]string{"type": "string"}},
						{"name": "event", "in": "query", "required": true, "schema": map[string]interface{}{"type": "string", "enum": []string{"single_push", "double_push", "triple_push", "long_push", "btn_down", "btn_up", "toggle", "on", "off"}}},
						{"name": "channel", "in": "query", "schema": map[string]string{"type": "integer"}},
						{"name": "value", "in": "query", "schema": map[string]string{"type": "string"}},
						{"name": "token", "in": "query", "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Accepted"},
						"400": map[string]interface{}{"description": "Invalid parameters"},
						"401": map[string]interface{}{"description": "Invalid webhook token"},
						"404": map[string]interface{}{"description": "Unknown device"},
					},
				},
			},
			"/api/health": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":  "Health check",
					"tags":     []string{"System"},
					"security": []map[string]interface{}{},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "OK"},
					},
				},
			},
			"/api/devices": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List all Shelly devices",
					"tags":    []string{"Devices"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "List of devices with current state"},
					},
				},
			},
			"/api/devices/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Get a single device",
					"tags":    []string{"Devices"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Device details"},
						"404": map[string]interface{}{"description": "Device not found"},
					},
				},
				"put": map[string]interface{}{
					"summary": "Control a device",
					"tags":    []string{"Devices"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"on":         map[string]string{"type": "boolean"},
										"brightness": map[string]string{"type": "number"},
										"position":   map[string]string{"type": "integer"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Updated device state"},
					},
				},
			},
			"/api/inputs": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List all Shelly inputs",
					"tags":    []string{"Inputs"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "List of inputs"},
					},
				},
			},
			"/api/mappings": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List all mappings",
					"tags":    []string{"Mappings"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "List of mappings"},
					},
				},
				"post": map[string]interface{}{
					"summary": "Create a mapping",
					"tags":    []string{"Mappings"},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Created mapping"},
					},
				},
			},
			"/api/mappings/{id}": map[string]interface{}{
				"put": map[string]interface{}{
					"summary": "Update a mapping",
					"tags":    []string{"Mappings"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Updated mapping"},
					},
				},
				"delete": map[string]interface{}{
					"summary": "Delete a mapping",
					"tags":    []string{"Mappings"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Deleted"},
					},
				},
			},
			"/api/mappings/export": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Export mappings as JSON",
					"tags":    []string{"Mappings"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "JSON file download"},
					},
				},
			},
			"/api/mappings/import": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Import mappings from JSON",
					"tags":    []string{"Mappings"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Import result"},
					},
				},
			},
			"/api/config": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Get current configuration",
					"tags":    []string{"Config"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Current config"},
					},
				},
				"put": map[string]interface{}{
					"summary": "Update configuration",
					"tags":    []string{"Config"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Config updated"},
					},
				},
			},
			"/api/backups": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List backups",
					"tags":    []string{"Backups"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "List of backups"},
					},
				},
				"post": map[string]interface{}{
					"summary": "Create a backup",
					"tags":    []string{"Backups"},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Backup created"},
					},
				},
			},
			"/api/backups/upload": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Upload a backup file",
					"tags":    []string{"Backups"},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Backup uploaded"},
					},
				},
			},
			"/api/backups/{id}": map[string]interface{}{
				"delete": map[string]interface{}{
					"summary": "Delete a backup",
					"tags":    []string{"Backups"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Deleted"},
					},
				},
			},
			"/api/backups/{id}/download": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Download a backup",
					"tags":    []string{"Backups"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Backup file"},
					},
				},
			},
			"/api/backups/{id}/restore": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Restore from backup",
					"tags":    []string{"Backups"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Backup restored"},
					},
				},
			},
			"/api/udp/test": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Send a test UDP message",
					"tags":    []string{"UDP"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Message sent"},
					},
				},
			},
			"/api/export/inputs": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Export Loxone Virtual Inputs XML",
					"tags":    []string{"Export"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "XML file"},
					},
				},
			},
			"/api/export/outputs": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Export Loxone Virtual Outputs XML",
					"tags":    []string{"Export"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "XML file"},
					},
				},
			},
			"/api/logs": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Get log entries",
					"tags":    []string{"System"},
					"parameters": []map[string]interface{}{
						{"name": "level", "in": "query", "schema": map[string]string{"type": "string"}},
						{"name": "search", "in": "query", "schema": map[string]string{"type": "string"}},
						{"name": "limit", "in": "query", "schema": map[string]string{"type": "integer"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Log entries"},
					},
				},
			},
			"/api/shelly/discover": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Discover Shelly devices on the network",
					"tags":    []string{"Shelly"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Discovered devices"},
					},
				},
			},
			"/api/shelly/add": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Add a new Shelly device",
					"tags":    []string{"Shelly"},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"id":         map[string]string{"type": "string"},
										"name":       map[string]string{"type": "string"},
										"ip":         map[string]string{"type": "string"},
										"generation": map[string]string{"type": "integer"},
									},
									"required": []string{"ip"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Device added"},
					},
				},
			},
			"/api/shelly/{id}": map[string]interface{}{
				"delete": map[string]interface{}{
					"summary": "Remove a Shelly device",
					"tags":    []string{"Shelly"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Device removed"},
					},
				},
			},
			"/api/shelly/{id}/test": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Test connection to a Shelly device",
					"tags":    []string{"Shelly"},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Connection test result"},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(spec); err != nil {
		log.Debug().Err(err).Msg("Failed to write swagger spec")
	}
}
