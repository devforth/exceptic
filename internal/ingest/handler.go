// Package ingest receives envelopes from Sentry SDKs at
// POST /api/{projectID}/envelope/ and stores their events and logs.
package ingest

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/server"
)

type Handler struct {
	logger   *slog.Logger
	projects config.ProjectList // read per request: projects can change while running
	limiter  *rateLimiter       // nil when both limits are off
	reading  *readBudget
	dropped  *DropCounter
	now      func() time.Time
	pipeline *Pipeline // takes accepted envelopes into its inbox and stores them later
}

// newHandler is the handler of NewPipeline, which sets its pipeline.
func newHandler(logger *slog.Logger, settings *config.Config, projects config.ProjectList) *Handler {
	handler := &Handler{
		logger:   logger,
		projects: projects,
		now:      time.Now,
		reading:  newReadBudget(MaxReadingBytes, MaxProjectReadingBytes),
		dropped:  newDropCounter(logger),
	}

	handler.limiter = newRateLimiter(settings.RateLimitEventsPerMinute, settings.RateLimitLogsPerMinute)

	return handler
}

// requestError is a rejection that is reported to the SDK with an HTTP status.
type requestError struct {
	status     int
	message    string
	retryAfter string // seconds, only for 429
	rateLimits string // X-Sentry-Rate-Limits of a 429; empty: every category
	incomplete bool   // the body did not arrive whole
}

func (err *requestError) Error() string { return err.message }

func reject(status int, format string, args ...any) error {
	return &requestError{status: status, message: fmt.Sprintf(format, args...)}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	project, known := handler.project(r)
	originAllowed := applyCORS(w, r, project, known)

	if r.Method == http.MethodOptions {
		if !originAllowed {
			writeError(w, http.StatusForbidden, "origin not allowed")
			return
		}

		w.WriteHeader(http.StatusOK)

		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")

		return
	}

	if !originAllowed {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}

	response, err := handler.ingest(w, r)

	if err != nil {
		var rejection *requestError

		if errors.As(err, &rejection) {
			handler.logger.Debug("envelope rejected", "path", r.URL.Path, "status", rejection.status, "reason", rejection.message)

			if rejection.retryAfter != "" {
				writeRateLimited(w, rejection)
				return
			}

			writeError(w, rejection.status, rejection.message)

			return
		}

		handler.logger.Error("ingest failed", "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")

		return
	}

	server.WriteJSON(w, http.StatusOK, response)
}

// project is the project of the request's path, if there is one.
func (handler *Handler) project(r *http.Request) (config.Project, bool) {
	projectID, err := strconv.Atoi(r.PathValue("projectID"))

	if err != nil {
		return config.Project{}, false
	}

	return handler.projects.List().ByID(projectID)
}

func (handler *Handler) ingest(w http.ResponseWriter, r *http.Request) (response map[string]string, err error) {
	projectID, err := strconv.Atoi(r.PathValue("projectID"))

	if err != nil {
		return nil, reject(http.StatusNotFound, "invalid project id")
	}

	project, known := handler.projects.List().ByID(projectID)

	// Usually the key is in the request. Behind a tunnel it is only in the
	// envelope header, its first line, which is all that is read before
	// authenticating. A brotli body can only be read whole, and only
	// sentry-python sends one, always with the key in the request.
	var body *requestBody

	key := publicKeyFromRequest(r)

	if key == "" && known && !isBrotli(r) {
		if body, err = openBody(w, r, handler.reading, projectID); err != nil {
			return nil, err
		}

		defer body.close()

		header, err := body.header(handler.reading, projectID)

		if err != nil {
			return nil, err
		}

		key = publicKeyFromDSN(sentry.ParseEnvelopeHeader(header).DSN, projectID)
	}

	if key == "" {
		return nil, reject(http.StatusUnauthorized, "missing sentry_key")
	}

	if !known || key != project.PublicKey {
		return nil, reject(http.StatusForbidden, "invalid project or key")
	}

	// A paused project's apps are answered as if stored: nothing of the body is read.
	if project.Paused {
		return map[string]string{}, nil
	}

	var envelope *sentry.Envelope

	// What the project's app sent and was refused is counted (refusals
	// before are anyone's).
	defer func() {
		if err != nil {
			handler.countRefused(project, err, envelope)
		}
	}()

	// Any request with the key connects the project: SDKs send a session at
	// start, long before an error.
	handler.pipeline.markContact(project.ID, key, handler.now())

	if body == nil {
		if body, err = openBody(w, r, handler.reading, projectID); err != nil {
			return nil, err
		}

		defer body.close()
	}

	data, err := body.readAll(handler.reading, projectID)

	if err != nil {
		return nil, err
	}

	if envelope, err = sentry.ParseEnvelope(data); err != nil {
		envelope = nil

		return nil, reject(http.StatusBadRequest, "%v", err)
	}

	handler.dropped.addReports(project, envelope, handler.now())

	// Spans and other dropped items neither count nor get a limit.
	if !storesItems(envelope) {
		return map[string]string{}, nil
	}

	if handler.limiter == nil {
		return handler.pipeline.accept(project, envelope, data)
	}

	return handler.acceptWithinLimits(w, project, envelope, data)
}

// acceptWithinLimits drops the items of a category over its limit and
// accepts the rest with 200; X-Sentry-Rate-Limits names the limited
// categories, so the SDK pauses only them. Nothing left to store is a 429.
func (handler *Handler) acceptWithinLimits(w http.ResponseWriter, project config.Project, envelope *sentry.Envelope, data []byte) (map[string]string, error) {
	now := handler.now()
	wanted := countItems(envelope)
	decision := handler.limiter.take(project.ID, wanted, now)

	if !decision.any() {
		response, err := handler.pipeline.accept(project, envelope, data)

		if err != nil {
			handler.limiter.refund(project.ID, wanted, now)
		}

		return response, err
	}

	handler.warnLimited(project, decision)

	for kind, limited := range decision.limited {
		if limited {
			handler.dropped.add(project, dropRateLimit, dropKind(kind), int64(wanted[kind]), now)
		}
	}

	withoutLimited(envelope, decision)

	retryAfter := strconv.Itoa(decision.retryAfter)

	if !storesItems(envelope) {
		return nil, &requestError{status: http.StatusTooManyRequests, message: "rate limit exceeded", retryAfter: retryAfter, rateLimits: decision.header()}
	}

	response, err := handler.pipeline.accept(project, envelope, envelope.Encode())

	if err != nil {
		handler.limiter.refund(project.ID, decision.counted(wanted), now)

		return nil, err
	}

	w.Header().Set("X-Sentry-Rate-Limits", decision.header())

	return response, nil
}

// countRefused counts what a refusal of the project's envelope dropped: its
// events and logs once it was read, otherwise the envelope. A rate limit's
// items are counted where they are left out.
func (handler *Handler) countRefused(project config.Project, err error, envelope *sentry.Envelope) {
	reason := dropBusy // the store failing too

	var rejection *requestError

	if errors.As(err, &rejection) {
		switch {
		case rejection.rateLimits != "":
			return
		case rejection == errOverShare:
			reason = dropQueueShare
		case rejection.status == http.StatusRequestEntityTooLarge:
			reason = dropTooLarge
		case rejection.incomplete:
			reason = dropIncomplete
		case rejection.status != http.StatusTooManyRequests:
			reason = dropInvalid
		}
	}

	if envelope == nil {
		handler.dropped.add(project, reason, dropEnvelopes, 1, handler.now())

		return
	}

	handler.dropped.addItems(project, reason, countItems(envelope), handler.now())
}

// warnLimited logs a category limited first in its minute: ingest is
// public, so a flood logs one line a minute, not one per envelope.
func (handler *Handler) warnLimited(project config.Project, decision rateDecision) {
	for kind, first := range decision.first {
		if first {
			handler.logger.Warn("rate limit reached, dropping until the minute ends", "project", project.Name, "items", categoryLabels[kind],
				"limit_per_minute", handler.limiter.limits[kind], "seconds", decision.retryAfter)
		}
	}
}

// storesItems reports whether the envelope has an event or a log, the only
// items that are stored.
func storesItems(envelope *sentry.Envelope) bool {
	for _, item := range envelope.Items {
		if item.Header.Type == sentry.ItemEvent || item.Header.Type == sentry.ItemLog {
			return true
		}
	}

	return false
}

// Errors use Sentry's {"detail": "..."} shape; SDKs log it on failure.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("X-Sentry-Error", message)
	server.WriteJSON(w, status, map[string]string{"detail": message})
}

// writeRateLimited tells the SDK to pause sending. X-Sentry-Rate-Limits is
// "retry_after:categories:scope"; empty categories mean all data types.
// SDKs that read it ignore Retry-After, which the others apply to everything.
func writeRateLimited(w http.ResponseWriter, rejection *requestError) {
	rateLimits := rejection.rateLimits

	if rateLimits == "" {
		rateLimits = pauseEverything(rejection.retryAfter)
	}

	w.Header().Set("Retry-After", rejection.retryAfter)
	w.Header().Set("X-Sentry-Rate-Limits", rateLimits)
	writeError(w, http.StatusTooManyRequests, rejection.message)
}
