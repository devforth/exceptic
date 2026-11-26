// Package config loads server settings from environment variables.
package config

import (
	"cmp"
	"fmt"
	"log/slog"
	"math"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/platform/logging"
)

// Project is a source of events (e.g. "backend", "frontend") with its own DSN.
// ID is the number at the end of the DSN and identifies the project's stored
// data, so it must never change once in use. Projects live in the database;
// the ones in PROJECTS are created there when missing and take their key from
// PROJECTS at every start.
type Project struct {
	ID        int
	Name      string
	PublicKey string
	// LogLevels is the project's own list of log levels to record; nil uses
	// Recording (an empty list records nothing).
	LogLevels *[]string
	// Color marks the project in lists and emails: "#rrggbb", set in the UI
	// or picked from ProjectPalette by id.
	Color string
	// DiskShare is the percent of MAX_DATABASE_SIZE the project may hold
	// before a cleanup of its own; 100 sets no limit of its own.
	DiskShare int
	// AllowedOrigins are the sites whose browsers may send to it (origins.go);
	// empty: any.
	AllowedOrigins []string
}

// DSN is what a Sentry SDK is configured with: the server address (base,
// see server.PublicURL) with the project's key as the user and its ID as the
// path. The scheme stays: SDKs cannot parse a DSN without it.
func (project Project) DSN(base *url.URL) string {
	address := *base
	address.User = url.User(project.PublicKey)

	return address.JoinPath(strconv.Itoa(project.ID)).String()
}

// ProjectPalette gives projects distinct colors until one is chosen in the
// UI, where its colors are the presets; all read well on light and dark backgrounds.
var ProjectPalette = []PaletteColor{
	{"Blue", "#2563eb"}, {"Pink", "#be185d"}, {"Green", "#15803d"}, {"Violet", "#7c3aed"}, {"Orange", "#c2410c"},
	{"Teal", "#0f766e"}, {"Indigo", "#4338ca"}, {"Amber", "#a16207"}, {"Cyan", "#0e7490"}, {"Fuchsia", "#a21caf"},
}

// PaletteColor is a color of the palette with its name.
type PaletteColor struct {
	Name  string
	Value string // #rrggbb
}

// DefaultProjectColor is the palette color of a project id.
func DefaultProjectColor(id int) string {
	return ProjectPalette[(max(id, 1)-1)%len(ProjectPalette)].Value
}

var colorRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// ValidColor: "#rrggbb" in lowercase, as the color input sends it. Colors go
// into CSS and email styles, so nothing else is accepted.
func ValidColor(color string) bool {
	return colorRe.MatchString(color)
}

// ProjectList gives the current projects. Projects can be added and changed
// in the UI while the server runs, so ingest, the API and the UI ask for the
// list on every use instead of keeping a copy (see package projects).
type ProjectList interface {
	List() Projects
}

// List returns the projects; a fixed list serves where a changing one is expected.
func (projects Projects) List() Projects {
	return projects
}

// ValidProjectName: lowercase letters, digits, '-' and '_', not digits only
// (used in URLs and filters, where digits are an ID), up to MaxProjectName.
func ValidProjectName(name string) bool {
	return len(name) <= MaxProjectName && projectNameRe.MatchString(name)
}

// ParseEmail is an entered email address as stored: lowercase, without a
// display name; ok false when it is none or longer than MaxEmail.
func ParseEmail(entered string) (string, bool) {
	parsed, err := mail.ParseAddress(strings.TrimSpace(entered))

	if err != nil || len(parsed.Address) > MaxEmail {
		return "", false
	}

	return strings.ToLower(parsed.Address), true
}

// validAdminEmail: ADMIN_EMAIL is an email or, as usernames were, letters,
// digits and . _ @ - (adminLoginRe), up to MaxEmail.
func validAdminEmail(value string) bool {
	_, isEmail := ParseEmail(value)

	return isEmail || (len(value) <= MaxEmail && adminLoginRe.MatchString(value))
}

// ValidPasswordLength is the length rule of every password, ADMIN_PASSWORD
// included, in characters.
func ValidPasswordLength(password string) bool {
	length := utf8.RuneCountInString(password)

	return length >= MinPasswordLength && length <= MaxPasswordLength
}

// Projects is a list of projects: the registry's, or PROJECTS as written.
type Projects []Project

// ByID returns the project with this ID. Stored data can outlive its project
// (a deleted one is purged in batches), so callers handle ok == false.
func (projects Projects) ByID(id int) (Project, bool) {
	for _, project := range projects {
		if project.ID == id {
			return project, true
		}
	}

	return Project{}, false
}

// Find returns the project given as an ID ("1") or a name ("backend"), the
// two ways a project is referred to in URLs.
func (projects Projects) Find(idOrName string) (Project, bool) {
	for _, project := range projects {
		if strconv.Itoa(project.ID) == idOrName || project.Name == idOrName {
			return project, true
		}
	}

	return Project{}, false
}

type Config struct {
	// AppName (APP_NAME) names this server in the UI, emails and to agents.
	AppName  string
	Port     int
	DataDir  string
	LogLevel slog.Level
	// LogFormat (LOG_FORMAT): auto (pretty in a terminal, text otherwise),
	// pretty, text or json; see package logging.
	LogFormat string
	Projects  Projects
	// RateLimitEventsPerMinute and RateLimitLogsPerMinute cap the events and
	// the logs a project may send per minute, a guard against abuse (an
	// error in a loop is capped per issue, EventsPerIssuePerMinute); 0 turns a limit off.
	RateLimitEventsPerMinute int
	RateLimitLogsPerMinute   int
	// APIToken authorizes REST API requests (Authorization: Bearer <token>).
	// Empty means the API is closed.
	APIToken string
	// AllowedIPs restricts the API and UI (not ingest or /health) to these
	// networks. Empty means any address.
	AllowedIPs []netip.Prefix
	// TrustedProxies are reverse proxies whose X-Forwarded-For header is used
	// to find the client address. Empty means the header is ignored.
	TrustedProxies []netip.Prefix
	// AdminEmail and AdminPassword create the superadmin at the first
	// start (while there is none); later they are not used. AdminEmail is
	// the login as given, an email or not.
	// An empty password means the UI is closed.
	AdminEmail    string
	AdminPassword string
	// SessionLifetime (SESSION_LIFETIME) is how long a UI login lasts; it
	// is not extended by use.
	SessionLifetime time.Duration
	// Notifications: where to send alerts about new and regressed issues.
	Notifications Notifications
	// Google: the Google sign-in (GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET).
	Google Google
	// Retention: how much data is kept (MAX_DATABASE_SIZE).
	Retention Retention
	// Recording: which event and log levels are stored by default.
	Recording Recording
}

// Defaults used when the corresponding environment variable is not set.
const (
	DefaultPort                     = 8000
	DefaultDataDir                  = "./data"
	DefaultLogLevel                 = slog.LevelInfo
	DefaultRateLimitEventsPerMinute = 10000
	DefaultRateLimitLogsPerMinute   = 10000
	// SDKLogBatch is the most logs the JavaScript SDK sends in one envelope; a
	// log limit below it drops every full batch whole.
	SDKLogBatch        = 100
	DefaultMemoryLimit = 200 << 20 // GOMEMLIMIT, the Go runtime's soft limit

	DefaultAdminEmail = "admin"

	DefaultSessionLifetime = 24 * time.Hour

	// MinAPITokenLength keeps tokens out of brute-force range.
	MinAPITokenLength = 32
	// MinPasswordLength applies to every user's password: logins are rate
	// limited, but the UI is often public.
	MinPasswordLength = 12
	// Users: the email (the login) and the display name; passwords up to
	// MaxPasswordLength.
	MaxEmail          = 254
	MaxDisplayName    = 100
	MaxPasswordLength = 256
	// MaxConnectionName is the longest name of a notification connection;
	// VerificationCodeDigits the length of an email confirmation code.
	MaxConnectionName      = 100
	VerificationCodeDigits = 6
	// MaxProjectName is the longest project name; MaxProjectID the largest
	// project id (stored as 32 bits).
	MaxProjectName = 64
	MaxProjectID   = math.MaxUint32
	// A project's public key has MinPublicKey to MaxPublicKey letters or digits.
	MinPublicKey = 16
	MaxPublicKey = 64
	// ProjectNamePattern: lowercase letters, digits, '-' and '_', starting
	// with a letter or digit, not digits only (that reads as a project ID).
	// Also the pattern attribute of the project form, hence the escaped '-'.
	ProjectNamePattern = `[a-z][a-z0-9_\-]*|[0-9][a-z0-9_\-]*[a-z_\-][a-z0-9_\-]*`
)

var (
	projectNameRe = regexp.MustCompile(`^(?:` + ProjectNamePattern + `)$`)
	adminLoginRe  = regexp.MustCompile(`^[A-Za-z0-9._@-]+$`)
	publicKeyRe   = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9]{%d,%d}$`, MinPublicKey, MaxPublicKey))
)

// Load reads the configuration from the process environment.
func Load() (*Config, error) {
	return loadFrom(os.Getenv)
}

// LimitMemory sets DefaultMemoryLimit unless GOMEMLIMIT is set; the load tests call it to measure what the server does.
func LimitMemory() {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(DefaultMemoryLimit)
	}
}

func loadFrom(getenv func(string) string) (*Config, error) {
	config := &Config{
		Port:                     DefaultPort,
		DataDir:                  DefaultDataDir,
		LogLevel:                 DefaultLogLevel,
		RateLimitEventsPerMinute: DefaultRateLimitEventsPerMinute,
		RateLimitLogsPerMinute:   DefaultRateLimitLogsPerMinute,
		AdminEmail:               DefaultAdminEmail,
		SessionLifetime:          DefaultSessionLifetime,
	}

	var err error

	if config.AppName, err = parseAppName(getenv("APP_NAME")); err != nil {
		return nil, err
	}

	if value := getenv("PORT"); value != "" {
		port, err := strconv.Atoi(value)

		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("PORT: invalid port %q", value)
		}

		config.Port = port
	}

	if value := getenv("DATA_DIR"); value != "" {
		config.DataDir = value
	}

	if value := getenv("LOG_LEVEL"); value != "" {
		if err := config.LogLevel.UnmarshalText([]byte(value)); err != nil {
			return nil, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}

	config.LogFormat = cmp.Or(strings.ToLower(getenv("LOG_FORMAT")), logging.FormatAuto)

	if !slices.Contains(logging.Formats, config.LogFormat) {
		return nil, fmt.Errorf("LOG_FORMAT: %q is not one of %s", config.LogFormat, strings.Join(logging.Formats, ", "))
	}

	for name, limit := range map[string]*int{
		"RATE_LIMIT_EVENTS_PER_MINUTE": &config.RateLimitEventsPerMinute,
		"RATE_LIMIT_LOGS_PER_MINUTE":   &config.RateLimitLogsPerMinute,
	} {
		if value := getenv(name); value != "" {
			parsed, err := strconv.Atoi(value)

			if err != nil || parsed < 0 {
				return nil, fmt.Errorf("%s: invalid value %q (0 or a positive integer)", name, value)
			}

			*limit = parsed
		}
	}

	if value := getenv("API_TOKEN"); value != "" {
		if len(value) < MinAPITokenLength {
			return nil, fmt.Errorf("API_TOKEN: must be at least %d characters (e.g. openssl rand -hex 32)", MinAPITokenLength)
		}

		config.APIToken = value
	}

	// ADMIN_USERNAME is its former name, read while ADMIN_EMAIL is not set (main warns).
	for _, name := range []string{"ADMIN_USERNAME", "ADMIN_EMAIL"} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			if !validAdminEmail(value) {
				return nil, fmt.Errorf("%s: invalid value %q (an email, or up to %d letters, digits and . _ @ -)", name, value, MaxEmail)
			}

			config.AdminEmail = value
		}
	}

	if value := getenv("ADMIN_PASSWORD"); value != "" {
		if !ValidPasswordLength(value) {
			return nil, fmt.Errorf("ADMIN_PASSWORD: must be %d to %d characters", MinPasswordLength, MaxPasswordLength)
		}

		config.AdminPassword = value
	}

	if value := strings.TrimSpace(getenv("SESSION_LIFETIME")); value != "" {
		if config.SessionLifetime, err = parseDuration(value); err != nil {
			return nil, fmt.Errorf("SESSION_LIFETIME: %w", err)
		}
	}

	notifications, err := parseNotifications(getenv)

	if err != nil {
		return nil, err
	}

	config.Notifications = notifications

	if config.Google, err = parseGoogle(getenv, notifications.PublicURL); err != nil {
		return nil, err
	}

	recording, err := parseRecording(getenv)

	if err != nil {
		return nil, err
	}

	config.Recording = recording

	retention, err := parseRetention(getenv)

	if err != nil {
		return nil, err
	}

	config.Retention = retention

	allowedIPs, err := parseNetworks(getenv("ALLOWED_IPS"))

	if err != nil {
		return nil, fmt.Errorf("ALLOWED_IPS: %w", err)
	}

	config.AllowedIPs = allowedIPs

	trustedProxies, err := parseNetworks(getenv("TRUSTED_PROXIES"))

	if err != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}

	config.TrustedProxies = trustedProxies

	projects, err := parseProjects(getenv("PROJECTS"))

	if err != nil {
		return nil, fmt.Errorf("PROJECTS: %w", err)
	}

	config.Projects = projects

	return config, nil
}

// parseProjects parses "id:name:key,id:name:key". Empty input means no projects.
func parseProjects(raw string) (Projects, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return nil, nil
	}

	var projects Projects

	seenIDs := map[int]bool{}
	seenNames := map[string]bool{}
	seenKeys := map[string]bool{}

	for index, entry := range strings.Split(raw, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")

		if len(parts) != 3 {
			return nil, fmt.Errorf("entry %d %q: expected id:name:public_key", index+1, entry)
		}

		rawID, name, key := parts[0], parts[1], parts[2]

		id, err := strconv.Atoi(rawID)

		if err != nil || id < 1 || id > MaxProjectID {
			return nil, fmt.Errorf("entry %d: invalid id %q (integer from 1 to %d)", index+1, rawID, MaxProjectID)
		}

		if seenIDs[id] {
			return nil, fmt.Errorf("duplicate project id %d", id)
		}

		if !ValidProjectName(name) {
			return nil, fmt.Errorf("entry %d: invalid name %q (up to %d lowercase letters, digits, '-', '_', not digits only)", index+1, name, MaxProjectName)
		}

		if !publicKeyRe.MatchString(key) {
			return nil, fmt.Errorf("project %q: public key must be 16-64 letters or digits", name)
		}

		if seenNames[name] {
			return nil, fmt.Errorf("duplicate project name %q", name)
		}

		if seenKeys[key] {
			return nil, fmt.Errorf("project %q: public key is already used by another project", name)
		}

		seenIDs[id] = true
		seenNames[name] = true
		seenKeys[key] = true
		projects = append(projects, Project{ID: id, Name: name, PublicKey: key})
	}

	return projects, nil
}

// parseNetworks parses "10.0.0.0/8, 203.0.113.7, ::1". A plain address means
// just that address (/32 or /128).
func parseNetworks(raw string) ([]netip.Prefix, error) {
	var networks []netip.Prefix

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)

		if entry == "" {
			continue
		}

		if strings.Contains(entry, "/") {
			network, err := netip.ParsePrefix(entry)

			if err != nil {
				return nil, fmt.Errorf("invalid network %q", entry)
			}

			// Clients are compared as plain IPv4, so an IPv4-mapped network is written as one.
			if network.Addr().Is4In6() {
				if network.Bits() < 96 {
					return nil, fmt.Errorf("invalid network %q: an IPv4-mapped network needs at least /96", entry)
				}

				network = netip.PrefixFrom(network.Addr().Unmap(), network.Bits()-96)
			}

			networks = append(networks, network.Masked())

			continue
		}

		address, err := netip.ParseAddr(entry)

		if err != nil {
			return nil, fmt.Errorf("invalid address %q", entry)
		}

		networks = append(networks, netip.PrefixFrom(address.Unmap(), address.Unmap().BitLen()))
	}

	return networks, nil
}

// parseDuration accepts days ("30d") as well as Go durations ("12h", "90m"),
// for SESSION_LIFETIME and the NOTIFY_* periods.
func parseDuration(value string) (time.Duration, error) {
	var (
		duration time.Duration
		err      error
	)

	if days, ok := strings.CutSuffix(value, "d"); ok {
		count, parseErr := strconv.ParseInt(days, 10, 64)

		if count < 1 || count > math.MaxInt64/int64(24*time.Hour) {
			parseErr = strconv.ErrRange
		}

		duration, err = time.Duration(count)*24*time.Hour, parseErr
	} else {
		duration, err = time.ParseDuration(value)
	}

	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid duration %q (e.g. 30d, 12h, 90m)", value)
	}

	return duration, nil
}

// HumanDuration writes a duration for people, in its largest whole unit:
// "15 minutes", "1 hour", "30 days" (or as Go writes it when none fits).
func HumanDuration(duration time.Duration) string {
	for _, unit := range []struct {
		size time.Duration
		name string
	}{{24 * time.Hour, "day"}, {time.Hour, "hour"}, {time.Minute, "minute"}, {time.Second, "second"}} {
		if duration <= 0 || duration%unit.size != 0 {
			continue
		}

		count := int(duration / unit.size)

		if count == 1 {
			return "1 " + unit.name
		}

		return strconv.Itoa(count) + " " + unit.name + "s"
	}

	return duration.String()
}
