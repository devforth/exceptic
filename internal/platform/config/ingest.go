package config

// Size limits of what ingest stores, beyond the cap of an envelope
// (docs/configuration.md, Size limits). A cut text keeps its first bytes, up
// to the limit and where a character starts, and ends with "…".
const (
	// MaxLogBodyBytes caps the body of a log.
	MaxLogBodyBytes = 16 << 10
	// MaxLogAttributes caps the attributes of a log: the "sentry." ones (the
	// parent span first) stay first, then the rest by name.
	MaxLogAttributes = 64
	// MaxLogAttributeBytes caps an attribute's value; a longer one of another
	// type than string is kept as its JSON text, cut.
	MaxLogAttributeBytes = 4 << 10

	// MaxEventStringBytes caps every string value of an event.
	MaxEventStringBytes = 16 << 10
	// MaxEventBytes is the size an event is cut to, least useful parts first;
	// TruncatedBreadcrumbs are the newest breadcrumbs the first cut keeps.
	MaxEventBytes        = 1 << 20
	TruncatedBreadcrumbs = 10
)
