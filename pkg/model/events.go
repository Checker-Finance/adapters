package model

import "time"

type TradeStatusEvent struct {
	TenantID  string    `json:"tenant_id"`
	ClientID  string    `json:"client_id"`
	DeskID    string    `json:"desk_id"`
	OrderID   string    `json:"order_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}
