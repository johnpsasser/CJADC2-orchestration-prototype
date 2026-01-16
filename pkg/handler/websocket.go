package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type          string          `json:"type"`
	Payload       json.RawMessage `json:"payload"`
	Timestamp     time.Time       `json:"timestamp"`
	CorrelationID string          `json:"correlation_id,omitempty"`
}

// MessageType constants
const (
	MessageTypeTrackUpdate    = "track.update"
	MessageTypeTrackNew       = "track.new"
	MessageTypeProposalNew    = "proposal.new"
	MessageTypeDecisionMade   = "decision.made"
	MessageTypeEffectExecuted = "effect.executed"
	MessageTypeMetricsUpdate  = "metrics.update"
	MessageTypePing           = "ping"
	MessageTypePong           = "pong"
	MessageTypeError          = "error"
)

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	id         string
	conn       *websocket.Conn
	send       chan WebSocketMessage
	hub        *WebSocketHub
	subscribed map[string]bool
	mu         sync.RWMutex
}

// WebSocketHub manages WebSocket connections and message broadcasting
type WebSocketHub struct {
	clients    map[string]*WebSocketClient
	broadcast  chan WebSocketMessage
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	mu         sync.RWMutex
	logger     zerolog.Logger
	nc         *nats.Conn
	subs       []*nats.Subscription
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub(nc *nats.Conn, logger zerolog.Logger) *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[string]*WebSocketClient),
		broadcast:  make(chan WebSocketMessage, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		logger:     logger.With().Str("component", "websocket_hub").Logger(),
		nc:         nc,
		subs:       make([]*nats.Subscription, 0),
	}
}

// Run starts the WebSocket hub
func (h *WebSocketHub) Run(ctx context.Context) {
	// Subscribe to NATS subjects
	if h.nc != nil {
		h.subscribeToNATS(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.id] = client
			h.mu.Unlock()
			h.logger.Info().Str("client_id", client.id).Int("total_clients", len(h.clients)).Msg("Client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.id]; ok {
				delete(h.clients, client.id)
				close(client.send)
			}
			h.mu.Unlock()
			h.logger.Info().Str("client_id", client.id).Int("total_clients", len(h.clients)).Msg("Client disconnected")

		case message := <-h.broadcast:
			h.mu.RLock()
			for _, client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client send buffer full, skip this message
					h.logger.Warn().Str("client_id", client.id).Str("message_type", message.Type).Msg("Client send buffer full, dropping message")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// subscribeToNATS subscribes to relevant NATS subjects
func (h *WebSocketHub) subscribeToNATS(ctx context.Context) {
	subjects := map[string]string{
		"track.>":             MessageTypeTrackUpdate,
		"proposal.pending.>":  MessageTypeProposalNew,
		"decision.>":          MessageTypeDecisionMade,
		"effect.>":            MessageTypeEffectExecuted,
	}

	for subject, msgType := range subjects {
		messageType := msgType // Capture for closure
		sub, err := h.nc.Subscribe(subject, func(msg *nats.Msg) {
			wsMsg := WebSocketMessage{
				Type:      messageType,
				Payload:   msg.Data,
				Timestamp: time.Now().UTC(),
			}

			// Try to extract correlation ID from the message
			var envelope struct {
				Envelope struct {
					CorrelationID string `json:"correlation_id"`
				} `json:"envelope"`
			}
			if err := json.Unmarshal(msg.Data, &envelope); err == nil {
				wsMsg.CorrelationID = envelope.Envelope.CorrelationID
			}

			// Distinguish between new and updated tracks
			if messageType == MessageTypeTrackUpdate && msg.Subject == "track.classified.unknown" {
				wsMsg.Type = MessageTypeTrackNew
			}

			select {
			case h.broadcast <- wsMsg:
			default:
				h.logger.Warn().Str("subject", msg.Subject).Msg("Broadcast buffer full, dropping message")
			}
		})

		if err != nil {
			h.logger.Error().Err(err).Str("subject", subject).Msg("Failed to subscribe to NATS subject")
			continue
		}

		h.subs = append(h.subs, sub)
		h.logger.Info().Str("subject", subject).Str("message_type", messageType).Msg("Subscribed to NATS subject")
	}
}

// shutdown cleanly shuts down the hub
func (h *WebSocketHub) shutdown() {
	// Unsubscribe from NATS
	for _, sub := range h.subs {
		sub.Unsubscribe()
	}

	// Close all client connections
	h.mu.Lock()
	for _, client := range h.clients {
		close(client.send)
	}
	h.clients = make(map[string]*WebSocketClient)
	h.mu.Unlock()

	h.logger.Info().Msg("WebSocket hub shutdown complete")
}

// Broadcast sends a message to all connected clients
func (h *WebSocketHub) Broadcast(msg WebSocketMessage) {
	select {
	case h.broadcast <- msg:
	default:
		h.logger.Warn().Str("message_type", msg.Type).Msg("Broadcast buffer full")
	}
}

// ClientCount returns the number of connected clients
func (h *WebSocketHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	hub    *WebSocketHub
	logger zerolog.Logger
}

// NewWebSocketHandler creates a new WebSocketHandler
func NewWebSocketHandler(hub *WebSocketHub, logger zerolog.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		hub:    hub,
		logger: logger.With().Str("handler", "websocket").Logger(),
	}
}

// ServeHTTP handles the WebSocket upgrade and connection
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:3000", "127.0.0.1:3000", "localhost:3001", "127.0.0.1:3001"},
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to accept WebSocket connection")
		return
	}

	clientID := uuid.New().String()
	client := &WebSocketClient{
		id:         clientID,
		conn:       conn,
		send:       make(chan WebSocketMessage, 64),
		hub:        h.hub,
		subscribed: make(map[string]bool),
	}

	h.hub.register <- client

	// Create context that cancels when connection closes
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Start writer and reader goroutines
	go client.writePump(ctx)
	client.readPump(ctx)
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *WebSocketClient) writePump(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case message, ok := <-c.send:
			if !ok {
				// Channel closed
				c.conn.Close(websocket.StatusNormalClosure, "connection closed")
				return
			}

			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := wsjson.Write(ctx, c.conn, message)
			cancel()

			if err != nil {
				c.hub.logger.Error().Err(err).Str("client_id", c.id).Msg("Failed to write message")
				return
			}

		case <-ticker.C:
			// Send ping
			pingMsg := WebSocketMessage{
				Type:      MessageTypePing,
				Timestamp: time.Now().UTC(),
			}

			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := wsjson.Write(ctx, c.conn, pingMsg)
			cancel()

			if err != nil {
				c.hub.logger.Error().Err(err).Str("client_id", c.id).Msg("Failed to send ping")
				return
			}
		}
	}
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *WebSocketClient) readPump(ctx context.Context) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		var msg WebSocketMessage
		err := wsjson.Read(ctx, c.conn, &msg)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			c.hub.logger.Debug().Err(err).Str("client_id", c.id).Msg("Read error")
			return
		}

		// Handle client messages
		switch msg.Type {
		case MessageTypePong:
			// Client responded to ping, connection is alive
			continue

		case "subscribe":
			// Handle subscription requests
			var subRequest struct {
				Topics []string `json:"topics"`
			}
			if err := json.Unmarshal(msg.Payload, &subRequest); err == nil {
				c.mu.Lock()
				for _, topic := range subRequest.Topics {
					c.subscribed[topic] = true
				}
				c.mu.Unlock()
			}

		case "unsubscribe":
			// Handle unsubscription requests
			var unsubRequest struct {
				Topics []string `json:"topics"`
			}
			if err := json.Unmarshal(msg.Payload, &unsubRequest); err == nil {
				c.mu.Lock()
				for _, topic := range unsubRequest.Topics {
					delete(c.subscribed, topic)
				}
				c.mu.Unlock()
			}

		default:
			c.hub.logger.Debug().Str("client_id", c.id).Str("type", msg.Type).Msg("Unknown message type")
		}
	}
}

// isSubscribed checks if the client is subscribed to a message type
func (c *WebSocketClient) isSubscribed(msgType string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// If no specific subscriptions, receive all messages
	if len(c.subscribed) == 0 {
		return true
	}

	return c.subscribed[msgType]
}
