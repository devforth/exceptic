package ingest

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

// MaxBodyBytes caps an envelope both before and after decompression.
// Events are usually a few KB; large ones carry local variables or breadcrumbs.
const MaxBodyBytes = 20 << 20

// MaxHeaderBytes caps the envelope header line, read before the key is known.
const MaxHeaderBytes = 64 << 10

// MaxReadingBytes caps the memory of all request bodies being read at once;
// the bodies of one project may take half of it.
const MaxReadingBytes = 4 * MaxBodyBytes

// BodyReadTimeout bounds reading a body, so a client that stops sending
// does not keep its part of the budget.
const BodyReadTimeout = 10 * time.Second

// requestBody streams the decompressed body, so the envelope header (its
// first line) can be read before the rest. SDKs don't always send
// Content-Type (Node sends none, the browser text/plain), so it is ignored.
type requestBody struct {
	stream  io.Reader
	read    []byte // what header read of the stream
	closers []func()
}

func openBody(w http.ResponseWriter, r *http.Request, budget *readBudget, projectID int) (*requestBody, error) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(BodyReadTimeout)) // not every writer supports it

	var (
		stream io.Reader = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		err    error
		body   = &requestBody{}
	)

	switch encoding := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); encoding {
	case "", "identity":
	case "gzip", "x-gzip":
		var decompressor *gzip.Reader

		if decompressor, err = gzip.NewReader(stream); err == nil {
			stream = decompressor

			body.closers = append(body.closers, func() { _ = decompressor.Close() }) // reading only: nothing to flush
		}
	case "deflate":
		var decompressor io.ReadCloser

		if decompressor, err = zlib.NewReader(stream); err == nil {
			stream = decompressor

			body.closers = append(body.closers, func() { _ = decompressor.Close() })
		}
	case "br": // sentry-python whenever the brotli module is installed
		if stream, err = body.brotli(stream, budget, projectID); err != nil {
			body.close()

			return nil, err
		}
	default:
		return nil, reject(http.StatusUnsupportedMediaType, "unsupported Content-Encoding %q (use gzip, deflate, br or none)", encoding)
	}

	if err != nil {
		return nil, readError(err)
	}

	body.stream = stream

	return body, nil
}

func isBrotli(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "br")
}

// brotli reads the compressed body whole, its buffer growing as bytes
// arrive, and only then holds the window the stream declares, which the
// decoder's ring buffer may fill before any output.
func (body *requestBody) brotli(stream io.Reader, budget *readBudget, projectID int) (io.Reader, error) {
	body.stream = stream

	compressed, err := body.fill(make([]byte, 0, 1<<10), MaxBodyBytes, budget, projectID, nil)

	if err != nil {
		return nil, err
	}

	if len(compressed) > 0 && !body.hold(budget, projectID, brotliWindow(compressed[0])) {
		return nil, errBusy
	}

	return brotli.NewReader(bytes.NewReader(compressed)), nil
}

// brotliWindow is the window size a brotli stream declares in its first byte
// (RFC 7932, section 9.1), at most 16 MB.
func brotliWindow(first byte) int {
	if first&1 == 0 {
		return 1 << 16
	}

	if bits := first >> 1 & 7; bits != 0 {
		return 1 << (17 + bits)
	}

	if bits := first >> 4 & 7; bits != 0 {
		return 1 << (8 + bits)
	}

	return 1 << 17
}

func (body *requestBody) close() {
	for _, closer := range body.closers {
		closer()
	}
}

// header reads up to the first newline, which is kept for readAll; the
// buffer grows only as bytes arrive.
func (body *requestBody) header(budget *readBudget, projectID int) ([]byte, error) {
	buffer, err := body.fill(make([]byte, 0, 1<<10), MaxHeaderBytes, budget, projectID, func(buffer []byte) bool {
		return bytes.IndexByte(buffer, '\n') >= 0
	})

	body.read = buffer

	if err != nil {
		return nil, err
	}

	if line, _, found := bytes.Cut(buffer, []byte("\n")); found {
		return line, nil
	}

	if len(buffer) > MaxHeaderBytes {
		return nil, reject(http.StatusRequestEntityTooLarge, "envelope header is larger than %d bytes", MaxHeaderBytes)
	}

	return buffer, nil // a header without items
}

// readAll reads the whole body, at most MaxBodyBytes after decompression.
func (body *requestBody) readAll(budget *readBudget, projectID int) ([]byte, error) {
	buffer := append(make([]byte, 0, max(32<<10, 2*len(body.read))), body.read...)
	buffer, err := body.fill(buffer, MaxBodyBytes, budget, projectID, nil)

	if err != nil {
		return nil, err
	}

	if len(buffer) > MaxBodyBytes {
		return nil, reject(http.StatusRequestEntityTooLarge, "envelope is larger than %d bytes", MaxBodyBytes)
	}

	return buffer, nil
}

// fill reads the stream into buffer until the end, until done says so, or
// one byte past limit, holding the buffer's capacity in the budget until the
// body is closed.
func (body *requestBody) fill(buffer []byte, limit int, budget *readBudget, projectID int, done func([]byte) bool) ([]byte, error) {
	if !body.hold(budget, projectID, cap(buffer)) {
		return nil, errBusy
	}

	for done == nil || !done(buffer) {
		if len(buffer) > limit {
			return buffer, nil
		}

		if len(buffer) == cap(buffer) {
			grown := min(2*cap(buffer), limit+1) // +1 byte tells "at the limit" from "over it"

			if !body.hold(budget, projectID, grown-cap(buffer)) {
				return nil, errBusy
			}

			buffer = append(make([]byte, 0, grown), buffer...)
		}

		read, err := body.stream.Read(buffer[len(buffer):cap(buffer)])
		buffer = buffer[:len(buffer)+read]

		if errors.Is(err, io.EOF) {
			return buffer, nil
		}

		if err != nil {
			return nil, readError(err)
		}
	}

	return buffer, nil
}

// hold reserves size bytes in the budget, released when the body is closed.
func (body *requestBody) hold(budget *readBudget, projectID, size int) bool {
	if !budget.reserve(projectID, int64(size)) {
		return false
	}

	body.closers = append(body.closers, func() { budget.release(projectID, int64(size)) })

	return true
}

// errCompacting: a compaction part of the database runs (seconds), during which nothing is written.
var errCompacting = &requestError{status: http.StatusTooManyRequests, message: "server is compacting its database", retryAfter: "5"}

// errBusy asks the SDK to retry in a second.
var errBusy = &requestError{status: http.StatusTooManyRequests, message: "server is busy reading other envelopes", retryAfter: "1"}

// readBudget counts the bytes of bodies being read, so a burst of large or
// compressed envelopes gets 429 instead of exhausting memory, and one
// project (whose key a browser app makes public) cannot take all of it.
type readBudget struct {
	mutex        sync.Mutex
	limit        int64
	projectLimit int64
	used         int64
	usedBy       map[int]int64
}

func newReadBudget(limit int64) *readBudget {
	return &readBudget{limit: limit, projectLimit: limit / 2, usedBy: map[int]int64{}}
}

func (budget *readBudget) reserve(projectID int, size int64) bool {
	budget.mutex.Lock()
	defer budget.mutex.Unlock()

	if budget.used+size > budget.limit || budget.usedBy[projectID]+size > budget.projectLimit {
		return false
	}

	budget.used += size
	budget.usedBy[projectID] += size

	return true
}

func (budget *readBudget) release(projectID int, size int64) {
	budget.mutex.Lock()
	defer budget.mutex.Unlock()

	budget.used -= size

	if budget.usedBy[projectID] -= size; budget.usedBy[projectID] == 0 {
		delete(budget.usedBy, projectID)
	}
}

func readError(err error) error {
	var tooLarge *http.MaxBytesError

	if errors.As(err, &tooLarge) {
		return reject(http.StatusRequestEntityTooLarge, "envelope is larger than %d bytes", MaxBodyBytes)
	}

	// The body did not arrive within BodyReadTimeout.
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return &requestError{status: http.StatusRequestTimeout, message: "read body: " + err.Error(), incomplete: true}
	}

	var network net.Error

	// The client hung up mid-body (or a compressed stream ends early).
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &network) {
		return &requestError{status: http.StatusBadRequest, message: "read body: " + err.Error(), incomplete: true}
	}

	return reject(http.StatusBadRequest, "read body: %v", err)
}

// publicKeyFromRequest returns the key from the "sentry_key" query parameter
// (JS SDKs) or from "X-Sentry-Auth: Sentry sentry_key=..., sentry_version=7"
// (most other SDKs; some send the same value in Authorization).
func publicKeyFromRequest(r *http.Request) string {
	if key := r.URL.Query().Get("sentry_key"); key != "" {
		return key
	}

	for _, header := range []string{"X-Sentry-Auth", "Authorization"} {
		value := strings.TrimSpace(r.Header.Get(header))

		if len(value) < len("Sentry ") || !strings.EqualFold(value[:len("Sentry ")], "Sentry ") {
			continue
		}

		for _, pair := range strings.Split(value[len("Sentry "):], ",") {
			name, keyValue, found := strings.Cut(strings.TrimSpace(pair), "=")

			if found && strings.TrimSpace(name) == "sentry_key" {
				return strings.TrimSpace(keyValue)
			}
		}
	}

	return ""
}

// publicKeyFromDSN returns the key from a DSN in the envelope header
// ("https://<key>@host/<projectID>"), which SDKs add when sending through a
// tunnel. The DSN must point at the same project as the request path.
func publicKeyFromDSN(dsn string, projectID int) string {
	if dsn == "" {
		return ""
	}

	parsed, err := url.Parse(dsn)

	if err != nil || parsed.User == nil {
		return ""
	}

	path := strings.TrimSuffix(parsed.Path, "/")

	if path[strings.LastIndexByte(path, '/')+1:] != strconv.Itoa(projectID) {
		return ""
	}

	return parsed.User.Username()
}
