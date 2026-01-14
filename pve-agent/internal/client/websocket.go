package client

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moconnor/pve-agent/internal/types"
)

// Client manages the WebSocket connection to pCenter
type Client struct {
	url      string
	token    string
	node     string
	cluster  string
	conn     *websocket.Conn
	mu       sync.Mutex
	handlers map[string]func(json.RawMessage)

	// Channels
	sendCh   chan *types.Message
	closeCh  chan struct{}
	closed   bool
}

// NewClient creates a new WebSocket client
func NewClient(url, token, node, cluster string) *Client {
	return &Client{
		url:      url,
		token:    token,
		node:     node,
		cluster:  cluster,
		handlers: make(map[string]func(json.RawMessage)),
		sendCh:   make(chan *types.Message, 100),
		closeCh:  make(chan struct{}),
	}
}

// OnMessage registers a handler for a message type
func (c *Client) OnMessage(msgType string, handler func(json.RawMessage)) {
	c.handlers[msgType] = handler
}

// Connect establishes the WebSocket connection
func (c *Client) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Add auth header
	headers := make(map[string][]string)
	if c.token != "" {
		headers["Authorization"] = []string{"Bearer " + c.token}
	}

	slog.Info("connecting to pCenter", "url", c.url)

	conn, _, err := dialer.DialContext(ctx, c.url, headers)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.closed = false
	c.closeCh = make(chan struct{}) // Reset for reconnection
	c.mu.Unlock()

	slog.Info("connected to pCenter")

	// Send registration message
	if err := c.register(); err != nil {
		conn.Close()
		return err
	}

	// Start read/write goroutines
	go c.readPump()
	go c.writePump()

	return nil
}

// register sends the registration message
func (c *Client) register() error {
	msg := &types.Message{
		Type:      types.MsgTypeRegister,
		Timestamp: time.Now().Unix(),
		Data: types.RegisterData{
			Node:    c.node,
			Cluster: c.cluster,
			Version: "1.0.0",
		},
	}

	return c.sendDirect(msg)
}

// Send queues a message to be sent
func (c *Client) Send(msg *types.Message) {
	select {
	case c.sendCh <- msg:
	default:
		slog.Warn("send channel full, dropping message")
	}
}

// SendStatus sends a status update
func (c *Client) SendStatus(status *types.StatusData) {
	msg := &types.Message{
		Type:      types.MsgTypeStatus,
		Timestamp: time.Now().Unix(),
		Data:      status,
	}
	c.Send(msg)
}

// sendDirect sends a message immediately (used for registration)
func (c *Client) sendDirect(msg *types.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	return c.conn.WriteJSON(msg)
}

// readPump handles incoming messages
func (c *Client) readPump() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil // Signal disconnection so IsConnected() returns false
		}
		c.mu.Unlock()
	}()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if !c.closed {
				slog.Error("read error", "error", err)
			}
			return
		}

		var msg struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Warn("failed to parse message", "error", err)
			continue
		}

		if handler, ok := c.handlers[msg.Type]; ok {
			handler(msg.Data)
		}
	}
}

// writePump handles outgoing messages
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer func() {
		// Signal disconnection on write pump exit
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
	}()

	for {
		select {
		case msg := <-c.sendCh:
			c.mu.Lock()
			if c.conn == nil {
				c.mu.Unlock()
				return // Exit if connection is gone
			}
			err := c.conn.WriteJSON(msg)
			c.mu.Unlock()
			if err != nil {
				slog.Error("write error", "error", err)
				return
			}

		case <-ticker.C:
			// Send heartbeat
			c.mu.Lock()
			if c.conn == nil {
				c.mu.Unlock()
				return // Exit if connection is gone
			}
			err := c.conn.WriteJSON(&types.Message{
				Type:      types.MsgTypeHeartbeat,
				Timestamp: time.Now().Unix(),
				Data: types.HeartbeatData{
					Node: c.node,
				},
			})
			c.mu.Unlock()
			if err != nil {
				slog.Error("heartbeat error", "error", err)
				return
			}

		case <-c.closeCh:
			return
		}
	}
}

// Close closes the connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.closeCh)

	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
	}
}

// IsConnected returns true if connected
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && !c.closed
}
