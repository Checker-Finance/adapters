package alphapoint

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// MessageHandler is called when a message is received
type MessageHandler func(response *Response)

// Client is a WebSocket client for AlphaPoint
type Client struct {
	url            string
	conn           *websocket.Conn
	logger         *zap.Logger
	sequence       int64
	handlers       []MessageHandler
	handlersMu     sync.RWMutex
	connected      bool
	connectedMu    sync.RWMutex
	done           chan struct{}
	reconnectDelay time.Duration
}

// NewClient creates a new AlphaPoint WebSocket client
func NewClient(url string, logger *zap.Logger) *Client {
	return &Client{
		url:            url,
		logger:         logger,
		handlers:       make([]MessageHandler, 0),
		done:           make(chan struct{}),
		reconnectDelay: 5 * time.Second,
	}
}

// Connect establishes a WebSocket connection
func (c *Client) Connect(ctx context.Context) error {
	c.logger.Info("Connecting to WebSocket", zap.String("url", c.url))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	c.conn = conn
	c.setConnected(true)
	c.logger.Info("Connected to WebSocket")

	// Start read loop
	go c.readLoop()

	return nil
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	close(c.done)
	c.setConnected(false)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.connectedMu.RLock()
	defer c.connectedMu.RUnlock()
	return c.connected
}

func (c *Client) setConnected(connected bool) {
	c.connectedMu.Lock()
	defer c.connectedMu.Unlock()
	c.connected = connected
}

// AddHandler adds a message handler
func (c *Client) AddHandler(handler MessageHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.handlers = append(c.handlers, handler)
}

// SendMessage sends a message to AlphaPoint
func (c *Client) SendMessage(ctx context.Context, operationName string, payload interface{}) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to WebSocket")
	}

	seq := int(atomic.AddInt64(&c.sequence, 2))
	request, err := NewRequest(MessageTypeRequest, seq, operationName, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	c.logger.Debug("Sending message",
		zap.String("operation", operationName),
		zap.Int("sequence", seq),
	)

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (c *Client) readLoop() {
	defer func() {
		c.setConnected(false)
		c.logger.Info("WebSocket read loop exited")
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Info("WebSocket closed normally")
					return
				}
				c.logger.Error("Error reading WebSocket message", zap.Error(err))
				c.scheduleReconnect()
				return
			}

			c.logger.Debug("Received message", zap.String("payload", string(message)))

			var response Response
			if err := json.Unmarshal(message, &response); err != nil {
				c.logger.Error("Failed to unmarshal response", zap.Error(err))
				continue
			}

			c.notifyHandlers(&response)
		}
	}
}

func (c *Client) notifyHandlers(response *Response) {
	c.handlersMu.RLock()
	defer c.handlersMu.RUnlock()

	for _, handler := range c.handlers {
		handler(response)
	}
}

func (c *Client) scheduleReconnect() {
	c.logger.Info("Scheduling reconnection", zap.Duration("delay", c.reconnectDelay))

	time.AfterFunc(c.reconnectDelay, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := c.Connect(ctx); err != nil {
			c.logger.Error("Reconnection failed", zap.Error(err))
			c.scheduleReconnect()
		}
	})
}

// Reconnect attempts to reconnect to the WebSocket
func (c *Client) Reconnect(ctx context.Context) error {
	if c.conn != nil {
		c.conn.Close()
	}
	return c.Connect(ctx)
}
