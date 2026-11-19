package contracts

import (
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/lists"
)

type DeliveryRow struct {
	store.OutboxRecord
	ConnectionID   string
	ConnectionName string
	Description    notify.Description
}

// Retrying: waiting for another attempt after a failed one.
func (row DeliveryRow) Retrying() bool {
	return row.Status == store.OutboxPending && row.Attempts > 0
}

// Sent: the message went out.
func (row DeliveryRow) Sent() bool {
	return row.Status == store.OutboxSent
}

type DeliveriesView struct {
	Connections []lists.Option // by id (Name); the names are in Labels
	Labels      map[string]string
	Connection  string
	Status      string
	Statuses    []string
	Rows        []DeliveryRow
	Pager       lists.Pager
	Filter      lists.FilterLinks
}
