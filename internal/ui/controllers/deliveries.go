package controllers

import (
	"maps"
	"net/http"
	"slices"

	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/contracts"
	"github.com/devforth/exceptic/internal/ui/lists"
	"github.com/devforth/exceptic/internal/ui/presenters"
)

// The delivery log: every notification message with its status, attempts and
// last error, so a broken mail server or chat shows up without opening the
// database. Sent messages are kept for NOTIFY_KEEP_SENT, failed ones for
// NOTIFY_KEEP_FAILED.

func (controller *Controllers) DeliveriesPage(w http.ResponseWriter, r *http.Request) {
	connections, err := controller.connections.List(r.Context())

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	view := contracts.DeliveriesView{Labels: presenters.ConnectionLabels(connections), Statuses: lists.DeliveryStatuses}
	query := lists.CleanListQuery(r.URL.Query(), lists.DeliveryListParams(slices.Collect(maps.Keys(view.Labels))), controller.projects)
	view.Connection, view.Status, view.Filter = query.Get("connection"), query.Get("status"), lists.NewFilterLinks(r, query)

	view.Connections = presenters.ConnectionOptions(connections, view.Connection)

	filter := store.OutboxFilter{Channel: view.Connection, Status: view.Status}
	total, err := controller.deliveries.Count(r.Context(), filter)

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	page, perPage := lists.ReadPaging(query)
	view.Pager = lists.NewPager(view.Filter, page, perPage, total, lists.DeliveryList)
	filter.Limit, filter.Offset = perPage, (view.Pager.Page-1)*perPage

	records, err := controller.deliveries.List(r.Context(), filter)

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	view.Rows = presenters.DeliveryRows(records, view.Labels)
	controller.renderList(w, r, "deliveries", lists.DeliveryList, view)
}
