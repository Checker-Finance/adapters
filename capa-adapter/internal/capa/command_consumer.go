package capa

import (
	natsio "github.com/nats-io/nats.go"

	intnats "github.com/Checker-Finance/adapters/internal/nats"
)

// CommandConsumer subscribes to NATS command subjects and routes them to the Capa service.
// Delegates to the shared internal/nats.CommandConsumer.
type CommandConsumer struct {
	*intnats.CommandConsumer
}

// NewCommandConsumer creates a CommandConsumer for the Capa adapter.
func NewCommandConsumer(nc *natsio.Conn, svc *Service) *CommandConsumer {
	return &CommandConsumer{intnats.NewCommandConsumer(nc, svc, "capa")}
}

// Ensure *Service satisfies the shared CommandService interface at compile time.
var _ intnats.CommandService = (*Service)(nil)
