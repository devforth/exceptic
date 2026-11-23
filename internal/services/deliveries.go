package services

import (
	"context"
	"errors"

	"github.com/devforth/exceptic/internal/storage/store"
)

// Deliveries reads the notification outbox for the delivery log: within an
// access, the messages of the connections it sees.
type Deliveries struct {
	store       store.Store
	connections *Connections
}

func newDeliveries(database store.Store, connections *Connections) *Deliveries {
	return &Deliveries{store: database, connections: connections}
}

// Count counts the messages that match filter (its Limit and Offset are ignored).
func (service *Deliveries) Count(ctx context.Context, access store.Access, filter store.OutboxFilter) (int, error) {
	filter, ok, err := service.scope(ctx, access, filter)

	if !ok {
		return 0, err
	}

	return service.store.CountOutbox(ctx, filter)
}

// List returns one page of the messages that match filter, newest first.
func (service *Deliveries) List(ctx context.Context, access store.Access, filter store.OutboxFilter) ([]store.OutboxRecord, error) {
	filter, ok, err := service.scope(ctx, access, filter)

	if !ok {
		return nil, err
	}

	return service.store.ListOutbox(ctx, filter)
}

// scope limits filter to access: the messages about its projects
// (store.OutboxMessage.ProjectIDs) of the connections it sees; messages of
// deleted connections, and those stored before messages had projects, are
// seen only with every project. ok is false when nothing can match. A
// connection asked for outside access is ErrHidden.
func (service *Deliveries) scope(ctx context.Context, access store.Access, filter store.OutboxFilter) (store.OutboxFilter, bool, error) {
	if access.All() {
		return filter, true, nil
	}

	if filter.Channel != "" {
		if _, err := service.connections.Get(ctx, access, filter.Channel); errors.Is(err, ErrHidden) {
			return filter, false, err
		}
	}

	connections, err := service.connections.List(ctx, access)
	filter.Access, filter.Channels = access, []string{}

	for _, connection := range connections {
		filter.Channels = append(filter.Channels, connection.ID)
	}

	return filter, err == nil && len(filter.Channels) > 0, err
}
