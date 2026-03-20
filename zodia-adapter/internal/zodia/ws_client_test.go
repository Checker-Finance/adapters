package zodia

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── WSURL ───────────────────────────────────────────────────────────────────

func TestWSURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://trade-uk.zodiamarkets.com", "wss://trade-uk.zodiamarkets.com/ws"},
		{"http://localhost:8080", "ws://localhost:8080/ws"},
		{"https://api.example.com", "wss://api.example.com/ws"},
		{"wss://already-ws.com", "wss://already-ws.com/ws"}, // no scheme match → appends /ws
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, WSURL(tt.input))
		})
	}
}

// ─── mockWSConn ──────────────────────────────────────────────────────────────

type mockWSConn struct {
	written   [][]byte
	toRead    [][]byte
	readIndex int
	closeErr  error
}

func (m *mockWSConn) WriteMessage(_ int, data []byte) error {
	m.written = append(m.written, data)
	return nil
}

func (m *mockWSConn) ReadMessage() (int, []byte, error) {
	if m.readIndex >= len(m.toRead) {
		return 0, nil, fmt.Errorf("no more messages")
	}
	data := m.toRead[m.readIndex]
	m.readIndex++
	return websocket.TextMessage, data, nil
}

func (m *mockWSConn) Close() error {
	return m.closeErr
}

// ─── WSClient.SendJSON ────────────────────────────────────────────────────────

func TestWSClient_SendJSON_TextMessage(t *testing.T) {
	client := NewWSClient()
	conn := &mockWSConn{}

	type payload struct {
		Action string `json:"action"`
		Value  int    `json:"value"`
	}

	err := client.SendJSON(conn, payload{Action: "test", Value: 42})
	require.NoError(t, err)
	require.Len(t, conn.written, 1)

	var decoded payload
	require.NoError(t, json.Unmarshal(conn.written[0], &decoded))
	assert.Equal(t, "test", decoded.Action)
	assert.Equal(t, 42, decoded.Value)
}

func TestWSClient_SendJSON_MarshalError(t *testing.T) {
	client := NewWSClient()
	conn := &mockWSConn{}
	// channels cannot be marshalled to JSON
	err := client.SendJSON(conn, make(chan int))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}

// ─── WSClient.ReadMessage ─────────────────────────────────────────────────────

func TestWSClient_ReadMessage_Success(t *testing.T) {
	client := NewWSClient()
	expected := []byte(`{"action":"price_update"}`)
	conn := &mockWSConn{toRead: [][]byte{expected}}

	data, err := client.ReadMessage(conn)
	require.NoError(t, err)
	assert.Equal(t, expected, data)
}

func TestWSClient_ReadMessage_Error(t *testing.T) {
	client := NewWSClient()
	conn := &mockWSConn{toRead: [][]byte{}} // no messages

	_, err := client.ReadMessage(conn)
	assert.Error(t, err)
}

// ─── WSClient.Dial ────────────────────────────────────────────────────────────

func TestWSClient_Dial_RealServer(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck
		// send a single message then close
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"hello"}`))
	}))
	defer srv.Close()

	// Convert http:// → ws://
	wsURL := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/ws"

	client := NewWSClient()
	conn, err := client.Dial(t.Context(), wsURL)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close() //nolint:errcheck

	// Read the hello message
	data, err := client.ReadMessage(conn)
	require.NoError(t, err)
	assert.Contains(t, string(data), "hello")
}

func TestWSClient_Dial_RefusedConnection(t *testing.T) {
	client := NewWSClient()
	// Port 1 is always refused
	_, err := client.Dial(t.Context(), "ws://localhost:1/ws")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "zodia: ws dial")
}
