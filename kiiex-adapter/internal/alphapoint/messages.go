package alphapoint

import "encoding/json"

// MessageType represents the type of AlphaPoint message
type MessageType int

const (
	MessageTypeRequest     MessageType = 0
	MessageTypeResponse    MessageType = 1
	MessageTypeSubscribe   MessageType = 2
	MessageTypeEvent       MessageType = 3
	MessageTypeUnsubscribe MessageType = 4
	MessageTypeError       MessageType = 5
)

// Request represents a request message to AlphaPoint
type Request struct {
	M int    `json:"m"` // Message type
	I int    `json:"i"` // Sequence number
	N string `json:"n"` // Operation name
	O string `json:"o"` // Payload (JSON string)
}

// NewRequest creates a new AlphaPoint request
func NewRequest(messageType MessageType, sequence int, operationName string, payload interface{}) (*Request, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Request{
		M: int(messageType),
		I: sequence,
		N: operationName,
		O: string(payloadJSON),
	}, nil
}

// Response represents a response message from AlphaPoint
type Response struct {
	M int    `json:"m"` // Message type
	I int    `json:"i"` // Sequence number
	N string `json:"n"` // Operation name
	O string `json:"o"` // Payload (JSON string)
}

// ParsePayload parses the payload into the given type
func (r *Response) ParsePayload(v interface{}) error {
	return json.Unmarshal([]byte(r.O), v)
}
