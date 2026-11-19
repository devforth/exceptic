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
	cors     cors
	limiter  *rateLimiter // nil when rate limiting is disabled
	reading  *readBudget
	now      func() time.Time
	pipeline *Pipeline // takes accepted envelopes into its inbox and stores them later
}

// newHandler is the handler of NewPipeline, which sets its pipeline.
func newHandler(logger *slog.Logger, settings *config.Config, projects config.ProjectList) *Handler {
	handler := &Handler{
		logger:   logger,
		projects: projects,
		cors:     newCORS(settings.AllowedOrigins),
		now:      time.Now,
		reading:  newReadBudget(MaxReadingBytes),
	}

	if settings.RateLimitPerMinute > 0 {
		handler.limiter = newRateLimiter(settings.RateLimitPerMinute)
	}

	return handler
}

// requestError is a rejection that is reported to the SDK with an HTTP status.
type requestError struct {
	status     int
	message    string
	retryAfter string // seconds, only for 429
}

func (err *requestError) Error() string { return err.message }

func reject(status int, format string, args ...any) error {
	return &requestError{status: status, message: fmt.Sprintf(format, args...)}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	originAllowed := handler.cors.apply(w, r)

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
				writeRateLimited(w, rejection.retryAfter, rejection.message)
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

func (handler *Handler) ingest(w http.ResponseWriter, r *http.Request) (map[string]string, error) {
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

	envelope, err := sentry.ParseEnvelope(data)

	if err != nil {
		return nil, reject(http.StatusBadRequest, "%v", err)
	}

	// A 429 stops every category in the SDK, so spans and other dropped
	// items must not use up the limit of errors.
	if !storesItems(envelope) {
		return map[string]string{}, nil
	}

	if handler.limiter != nil {
		if retryAfter, allowed := handler.limiter.allow(projectID, handler.now()); !allowed {
			return nil, &requestError{status: http.StatusTooManyRequests, message: "rate limit exceeded", retryAfter: retryAfter}
		}
	}

	return handler.pipeline.accept(project, envelope, data)
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
func writeRateLimited(w http.ResponseWriter, retryAfter, message string) {
	w.Header().Set("Retry-After", retryAfter)
	w.Header().Set("X-Sentry-Rate-Limits", retryAfter+"::project")
	writeError(w, http.StatusTooManyRequests, message)
}
