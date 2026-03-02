package zodia

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WSConn is the interface for a WebSocket connection, enabling mock-based testing.
type WSConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// wsConnAdapter wraps a gorilla *websocket.Conn to implement WSConn.
type wsConnAdapter struct {
	conn *websocket.Conn
}

func (w *wsConnAdapter) WriteMessage(messageType int, data []byte) error {
	return w.conn.WriteMessage(messageType, data)
}

func (w *wsConnAdapter) ReadMessage() (int, []byte, error) {
	return w.conn.ReadMessage()
}

func (w *wsConnAdapter) Close() error {
	return w.conn.Close()
}

// WSClient provides low-level WebSocket send/receive operations for Zodia.
type WSClient struct {
	logger   *zap.Logger
	dialer   *websocket.Dialer
}

// NewWSClient constructs a new WSClient.
func NewWSClient(logger *zap.Logger) *WSClient {
	return &WSClient{
		logger: logger,
		dialer: &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

// Dial connects to the Zodia WebSocket server at the given URL.
// Returns a WSConn that implements the WSConn interface.
func (c *WSClient) Dial(ctx context.Context, url string) (WSConn, error) {
	reqHeader := http.Header{}
	conn, _, err := c.dialer.DialContext(ctx, url, reqHeader)
	if err != nil {
		return nil, fmt.Errorf("zodia: ws dial %q: %w", url, err)
	}
	c.logger.Info("zodia.ws.connected", zap.String("url", url))
	return &wsConnAdapter{conn: conn}, nil
}

// SendJSON marshals v to JSON and sends it as a WebSocket text message.
func (c *WSClient) SendJSON(conn WSConn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("zodia: ws marshal: %w", err)
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// ReadMessage reads the next message from the WebSocket connection and returns raw bytes.
func (c *WSClient) ReadMessage(conn WSConn) ([]byte, error) {
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return data, nil
}

// WSURL builds the WebSocket URL from a base HTTP URL.
// E.g. "https://trade-uk.zodiamarkets.com" → "wss://trade-uk.zodiamarkets.com/ws"
// ⚠️ Exact WS path ("/ws") needs verification against sandbox.
func WSURL(baseURL string) string {
	// Replace https/http scheme with wss/ws
	wsBase := baseURL
	if len(wsBase) >= 8 && wsBase[:8] == "https://" {
		wsBase = "wss://" + wsBase[8:]
	} else if len(wsBase) >= 7 && wsBase[:7] == "http://" {
		wsBase = "ws://" + wsBase[7:]
	}
	return wsBase + "/ws"
}
