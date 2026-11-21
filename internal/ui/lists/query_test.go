package lists

import (
	"net/url"
	"testing"
)

// The delivery log keeps known connections and statuses, and valid paging only.
func TestCleanDeliveryQuery(t *testing.T) {
	raw := url.Values{"connection": {"con00001"}, "status": {"bogus"}, "page": {"-3"}, "per_page": {"abc"}, "project": {"backend"}, "from": {"2026-11-01"}}
	query := CleanListQuery(raw, DeliveryListParams([]string{"con00001"}))

	if want := (url.Values{"connection": {"con00001"}}); query.Encode() != want.Encode() {
		t.Errorf("delivery query = %v, want %v", query, want)
	}

	if paged := CleanListQuery(url.Values{"page": {"3"}, "per_page": {"50"}}, DeliveryListParams(nil)); paged.Encode() != "page=3&per_page=50" {
		t.Errorf("paging = %v, want it kept", paged)
	}
}
