package ingest

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
)

// applyCORS sets the CORS response headers of a browser's request and
// reports whether its origin may send to the project (config.Project.OriginAllowed).
// Backend SDKs send no Origin header and are not affected. An unknown project
// answers any origin, so the SDK can read why it is refused.
func applyCORS(w http.ResponseWriter, r *http.Request, project config.Project, known bool) bool {
	header := w.Header()
	header.Add("Vary", "Origin")

	origin := r.Header.Get("Origin")

	if origin == "" {
		return true
	}

	if known && !project.OriginAllowed(origin) {
		return false
	}

	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Expose-Headers", "X-Sentry-Error, X-Sentry-Rate-Limits, Retry-After")

	if r.Method == http.MethodOptions {
		header.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		header.Set("Access-Control-Allow-Headers", "Content-Type, Content-Encoding, X-Sentry-Auth, Authorization, Sentry-Trace, Baggage")
		header.Set("Access-Control-Max-Age", "3600")
	}

	return true
}

// category is what a rate limit counts.
type category int

const (
	categoryEvents category = iota
	categoryLogs
	categoryCount
)

// itemCounts holds a number per category.
type itemCounts [categoryCount]int

// sdkCategories names each category as SDKs read X-Sentry-Rate-Limits: the
// JS SDK maps every event item to error, Sentry limits default and security
// with it; logs are log_item.
var sdkCategories = [categoryCount]string{"error;default;security", "log_item"}

var categoryLabels = [categoryCount]string{"events", "logs"}

// rateLimiter allows up to a number of events and of logs per project per
// calendar minute. A fixed window is enough here: the goal is to stop a flood
// (a log call in a loop, an error in a render loop on many clients), not to
// shape traffic precisely. Once a category is over, it stays closed until the
// minute ends, as the SDK is told to pause it that long. Projects are only
// those of the registry (checked before), so the map stays small.
type rateLimiter struct {
	mutex   sync.Mutex
	limits  itemCounts // 0: no limit
	windows map[int]*rateWindow
}

type rateWindow struct {
	start  time.Time
	counts itemCounts
	closed [categoryCount]bool
}

// rateDecision is what take found: the categories limited, those limited
// first in this window (worth a warning) and the seconds to the next window.
type rateDecision struct {
	limited    [categoryCount]bool
	first      [categoryCount]bool
	retryAfter int
}

// newRateLimiter returns nil when both limits are off.
func newRateLimiter(events, logs int) *rateLimiter {
	if events == 0 && logs == 0 {
		return nil
	}

	return &rateLimiter{limits: itemCounts{categoryEvents: events, categoryLogs: logs}, windows: map[int]*rateWindow{}}
}

// take counts the items of an envelope; a category over its limit is not
// counted, and the envelope's items of it are to be dropped whole.
func (limiter *rateLimiter) take(projectID int, wanted itemCounts, now time.Time) rateDecision {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	window := limiter.window(projectID, now)
	decision := rateDecision{retryAfter: min(int(window.start.Add(time.Minute).Sub(now).Seconds())+1, 61)}

	for kind, count := range wanted {
		limit := limiter.limits[kind]

		if count == 0 || limit == 0 {
			continue
		}

		if window.closed[kind] || window.counts[kind]+count > limit {
			decision.limited[kind] = true
			decision.first[kind] = !window.closed[kind]
			window.closed[kind] = true

			continue
		}

		window.counts[kind] += count
	}

	return decision
}

// refund takes back what take counted for an envelope that was not accepted.
func (limiter *rateLimiter) refund(projectID int, counted itemCounts, now time.Time) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	window := limiter.windows[projectID]

	// A window of a later minute has no counts of this one to take back.
	if window == nil || !window.start.Equal(now.Truncate(time.Minute)) {
		return
	}

	for kind, count := range counted {
		window.counts[kind] = max(window.counts[kind]-count, 0)
	}
}

// window is the project's window for now; a request that read the time
// before another opened the next minute counts in that newer window, so a
// window never moves back.
func (limiter *rateLimiter) window(projectID int, now time.Time) *rateWindow {
	start := now.Truncate(time.Minute)
	window := limiter.windows[projectID]

	if window == nil || window.start.Before(start) {
		window = &rateWindow{start: start}
		limiter.windows[projectID] = window
	}

	return window
}

func (decision rateDecision) any() bool {
	return slices.Contains(decision.limited[:], true)
}

// counted is what take counted of wanted.
func (decision rateDecision) counted(wanted itemCounts) itemCounts {
	for kind := range wanted {
		if decision.limited[kind] {
			wanted[kind] = 0
		}
	}

	return wanted
}

// header is the X-Sentry-Rate-Limits value naming only the limited
// categories ("retry_after:categories:scope"), so the SDK pauses only them.
func (decision rateDecision) header() string {
	var names []string

	for kind, limited := range decision.limited {
		if limited {
			names = append(names, sdkCategories[kind])
		}
	}

	return strconv.Itoa(decision.retryAfter) + ":" + strings.Join(names, ";") + ":project"
}

// countItems counts what the limits count: each event, and each log of a log
// item (a log item without logs as one, so empty items cannot flood the inbox).
func countItems(envelope *sentry.Envelope) itemCounts {
	var counts itemCounts

	for _, item := range envelope.Items {
		switch item.Header.Type {
		case sentry.ItemEvent:
			counts[categoryEvents]++
		case sentry.ItemLog:
			counts[categoryLogs] += max(logsInItem(item.Payload), 1)
		}
	}

	return counts
}

// logsInItem counts the elements of the payload's "items" (in any case, as
// sentry.ParseLogs reads it) without decoding them; the SDK's item_count is
// not trusted.
func logsInItem(payload []byte) int {
	decoder := newTokenDecoder(payload)

	if token, err := decoder.ReadToken(); err != nil || token.Kind() != '{' {
		return 0
	}

	count := 0

	for decoder.PeekKind() == '"' {
		name, err := decoder.ReadToken()

		if err != nil {
			return count
		}

		if sentry.FoldedName(name.String()) != "items" || decoder.PeekKind() != '[' {
			if decoder.SkipValue() != nil {
				return count
			}

			continue
		}

		if _, err := decoder.ReadToken(); err != nil {
			return count
		}

		for kind := decoder.PeekKind(); kind != ']' && kind != 0; kind = decoder.PeekKind() {
			if decoder.SkipValue() != nil {
				return count
			}

			count++
		}

		if _, err := decoder.ReadToken(); err != nil {
			return count
		}
	}

	return count
}

// withoutLimited leaves out the envelope's items of the limited categories.
func withoutLimited(envelope *sentry.Envelope, decision rateDecision) {
	envelope.Items = slices.DeleteFunc(envelope.Items, func(item sentry.Item) bool {
		return item.Header.Type == sentry.ItemEvent && decision.limited[categoryEvents] ||
			item.Header.Type == sentry.ItemLog && decision.limited[categoryLogs]
	})
}
