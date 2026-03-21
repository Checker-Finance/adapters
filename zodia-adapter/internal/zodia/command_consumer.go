package zodia

import (
	natsio "github.com/nats-io/nats.go"

	intnats "github.com/Checker-Finance/adapters/internal/nats"
)

// CommandConsumer subscribes to NATS command subjects and routes them to the Zodia service.
// Delegates to the shared internal/nats.CommandConsumer.
type CommandConsumer struct {
	*intnats.CommandConsumer
}

// NewCommandConsumer creates a CommandConsumer for the Zodia adapter.
func NewCommandConsumer(nc *natsio.Conn, svc *Service) *CommandConsumer {
	return &CommandConsumer{intnats.NewCommandConsumer(nc, svc, "zodia")}
}

// Ensure *Service satisfies the shared CommandService interface at compile time.
var _ intnats.CommandService = (*Service)(nil)
