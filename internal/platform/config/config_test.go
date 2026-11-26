package config

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestLoadDefaults(t *testing.T) {
	config, err := loadFrom(env(nil))

	if err != nil {
		t.Fatal(err)
	}

	if config.Port != DefaultPort || config.DataDir != DefaultDataDir || config.LogLevel != DefaultLogLevel || config.Projects != nil || config.APIToken != "" || config.AllowedIPs != nil ||
		config.RateLimitEventsPerMinute != 10000 || config.RateLimitLogsPerMinute != 10000 ||
		config.AdminEmail != DefaultAdminEmail || config.AdminPassword != "" || config.SessionLifetime != 24*time.Hour {
		t.Fatalf("unexpected defaults: %+v", config)
	}
}

func TestLoadValues(t *testing.T) {
	config, err := loadFrom(env(map[string]string{
		"PORT":                         "9000",
		"DATA_DIR":                     "/data",
		"LOG_LEVEL":                    "debug",
		"RATE_LIMIT_EVENTS_PER_MINUTE": "0",
		"RATE_LIMIT_LOGS_PER_MINUTE":   "500",
		// Replaced by the two above: ignored, whatever its value (main warns).
		"RATE_LIMIT_PER_MINUTE": "-1",
		"API_TOKEN":             "0123456789abcdef0123456789abcdef",
		"ALLOWED_IPS":           "10.0.0.0/8, 203.0.113.7,::1",
		"TRUSTED_PROXIES":       "172.17.0.1, 10.0.0.0/8",
		"ADMIN_EMAIL":           " root ", // logins are trimmed, so the stored one is too
		"ADMIN_PASSWORD":        "root-password",
		"SESSION_LIFETIME":      "12h",
		// IDs are explicit, so they don't have to follow list order or be contiguous.
		"PROJECTS": "3:frontend:0123456789abcdef0123, 1:backend:a1b2c3d4e5f6a7b8",
	}))

	if err != nil {
		t.Fatal(err)
	}

	if config.Port != 9000 || config.DataDir != "/data" || config.LogLevel != slog.LevelDebug || config.SessionLifetime != 12*time.Hour {
		t.Fatalf("unexpected config: %+v", config)
	}

	if config.RateLimitEventsPerMinute != 0 || config.RateLimitLogsPerMinute != 500 {
		t.Fatalf("rate limits = %d events, %d logs; want 0 (off), 500", config.RateLimitEventsPerMinute, config.RateLimitLogsPerMinute)
	}

	if config.APIToken != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("api token = %q", config.APIToken)
	}

	if config.AdminEmail != "root" || config.AdminPassword != "root-password" {
		t.Fatalf("admin = %q / %q", config.AdminEmail, config.AdminPassword)
	}

	if fmt.Sprint(config.AllowedIPs) != "[10.0.0.0/8 203.0.113.7/32 ::1/128]" || fmt.Sprint(config.TrustedProxies) != "[172.17.0.1/32 10.0.0.0/8]" {
		t.Fatalf("allowed=%v trusted=%v", config.AllowedIPs, config.TrustedProxies)
	}

	want := []Project{
		{ID: 3, Name: "frontend", PublicKey: "0123456789abcdef0123"},
		{ID: 1, Name: "backend", PublicKey: "a1b2c3d4e5f6a7b8"},
	}

	if len(config.Projects) != len(want) {
		t.Fatalf("got %d projects, want %d", len(config.Projects), len(want))
	}

	for index := range want {
		if !reflect.DeepEqual(config.Projects[index], want[index]) {
			t.Errorf("project %d = %+v, want %+v", index, config.Projects[index], want[index])
		}
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name, key, value, wantErr string
	}{
		{"bad port", "PORT", "abc", "PORT"},
		{"port out of range", "PORT", "70000", "PORT"},
		{"bad log level", "LOG_LEVEL", "loud", "LOG_LEVEL"},
		{"bad log format", "LOG_FORMAT", "fancy", "LOG_FORMAT"},
		{"old format without id", "PROJECTS", "backend:a1b2c3d4e5f6a7b8", "expected id:name:public_key"},
		{"too many parts", "PROJECTS", "1:backend:a1b2c3d4e5f6a7b8:x", "expected id:name:public_key"},
		{"non-numeric id", "PROJECTS", "x:backend:a1b2c3d4e5f6a7b8", "invalid id"},
		{"zero id", "PROJECTS", "0:backend:a1b2c3d4e5f6a7b8", "invalid id"},
		{"id above 32 bits", "PROJECTS", "4294967296:backend:a1b2c3d4e5f6a7b8", "invalid id"},
		{"bad name", "PROJECTS", "1:Back End:a1b2c3d4e5f6a7b8", "invalid name"},
		{"long name", "PROJECTS", "1:" + strings.Repeat("b", MaxProjectName+1) + ":a1b2c3d4e5f6a7b8", "invalid name"},
		{"short key", "PROJECTS", "1:backend:abc", "16-64"},
		{"duplicate id", "PROJECTS", "1:backend:a1b2c3d4e5f6a7b8,1:frontend:0123456789abcdef", "duplicate project id"},
		{"duplicate name", "PROJECTS", "1:backend:a1b2c3d4e5f6a7b8,2:backend:0123456789abcdef", "duplicate project name"},
		{"negative events rate limit", "RATE_LIMIT_EVENTS_PER_MINUTE", "-1", "RATE_LIMIT_EVENTS_PER_MINUTE"},
		{"bad logs rate limit", "RATE_LIMIT_LOGS_PER_MINUTE", "10k", "RATE_LIMIT_LOGS_PER_MINUTE"},
		{"short api token", "API_TOKEN", "secret", "at least"},
		{"short admin password", "ADMIN_PASSWORD", "short-pass", "12 to 256"},
		{"long admin password", "ADMIN_PASSWORD", strings.Repeat("p", MaxPasswordLength+1), "12 to 256"},
		{"admin email with a space", "ADMIN_EMAIL", "john doe", "ADMIN_EMAIL"},
		{"long admin email", "ADMIN_EMAIL", strings.Repeat("a", MaxEmail+1), "ADMIN_EMAIL"},
		{"former admin variable with a space", "ADMIN_USERNAME", "john doe", "ADMIN_USERNAME"},
		{"bad session lifetime", "SESSION_LIFETIME", "forever", "SESSION_LIFETIME"},
		{"zero session lifetime", "SESSION_LIFETIME", "0", "SESSION_LIFETIME"},
		{"session lifetime past the largest duration", "SESSION_LIFETIME", "213504d", "SESSION_LIFETIME"},
		{"session lifetime below the smallest duration", "SESSION_LIFETIME", "-106752d", "SESSION_LIFETIME"},
		{"bad allowed ip", "ALLOWED_IPS", "10.0.0.300", "invalid address"},
		{"bad network", "TRUSTED_PROXIES", "10.0.0.0/40", "invalid network"},
		{"short ipv4-mapped network", "ALLOWED_IPS", "::ffff:0:0/90", "IPv4-mapped"},
		{"duplicate key", "PROJECTS", "1:backend:a1b2c3d4e5f6a7b8,2:frontend:a1b2c3d4e5f6a7b8", "already used"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := loadFrom(env(map[string]string{testCase.key: testCase.value}))

			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("got error %v, want it to contain %q", err, testCase.wantErr)
			}
		})
	}
}

func TestProjectsLookup(t *testing.T) {
	projects := Projects{{ID: 3, Name: "frontend"}, {ID: 1, Name: "backend"}}

	for _, idOrName := range []string{"1", "backend"} {
		if project, ok := projects.Find(idOrName); !ok || project.ID != 1 {
			t.Errorf("Find(%q) = %+v, %v", idOrName, project, ok)
		}
	}

	if _, ok := projects.Find("billing"); ok {
		t.Error("Find of an unknown project must fail")
	}

	if project, ok := projects.ByID(3); !ok || project.Name != "frontend" {
		t.Errorf("ByID(3) = %+v, %v", project, ok)
	}

	if _, ok := projects.ByID(2); ok {
		t.Error("ByID of an unknown project must fail")
	}
}

// The ingest queue gets a fifth of the size limit, of the default one when the limit is off.
func TestMaxInboxBytes(t *testing.T) {
	if got := MaxInboxBytes(1000); got != 200 {
		t.Errorf("of 1000: %d", got)
	}

	if got := MaxInboxBytes(0); got != DefaultMaxDatabaseBytes/5 {
		t.Errorf("with no limit: %d", got)
	}
}

// Clients are compared as plain IPv4, so IPv4-mapped entries are stored as IPv4.
func TestMappedNetworks(t *testing.T) {
	networks, err := parseNetworks("::ffff:10.0.0.0/104, ::ffff:203.0.113.7, ::ffff:198.51.100.9/128")

	if err != nil {
		t.Fatal(err)
	}

	if got := fmt.Sprint(networks); got != "[10.0.0.0/8 203.0.113.7/32 198.51.100.9/32]" {
		t.Errorf("got %s", got)
	}
}

// ADMIN_EMAIL takes an email; ADMIN_USERNAME, its former name, is read while it is not set.
func TestAdminEmail(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"email", map[string]string{"ADMIN_EMAIL": "ops+exceptic@example.com"}, "ops+exceptic@example.com"},
		{"former name", map[string]string{"ADMIN_USERNAME": "root"}, "root"},
		{"both", map[string]string{"ADMIN_USERNAME": "root", "ADMIN_EMAIL": "admin@example.com"}, "admin@example.com"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := loadFrom(func(name string) string { return test.env[name] })

			if err != nil {
				t.Fatal(err)
			}

			if config.AdminEmail != test.want {
				t.Fatalf("AdminEmail = %q, want %q", config.AdminEmail, test.want)
			}
		})
	}
}
