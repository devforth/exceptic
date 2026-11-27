package docs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/ingest"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/secret"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
)

type value = fixtures.DocValue

// TestDocsValues: the numbers, defaults and lists the documents state come
// from the code (package tests check the values their package keeps to itself).
func TestDocsValues(t *testing.T) {
	notifyLevels := strings.Join(config.NotifyLevels, "` and `")
	diskFactor := fmt.Sprintf("%g", 1+float64(config.StalePercent+config.InboxPercent)/100)

	fixtures.DocValues(t, "docs/configuration.md",
		value{Phrase: "| `APP_NAME` | `{}` |", Value: config.DefaultAppName},
		value{Phrase: "Up to {} characters.", Value: config.MaxAppName},
		value{Phrase: "| `PORT` | `{}` |", Value: config.DefaultPort},
		value{Phrase: "| `DATA_DIR` | `{}` (", Value: config.DefaultDataDir},
		value{Phrase: "| `LOG_LEVEL` | `{}` |", Value: strings.ToLower(config.DefaultLogLevel.String())},
		value{Phrase: "| `GOMEMLIMIT` | `{}MiB` |", Value: config.DefaultMemoryLimit >> 20},
		value{Phrase: "| `ADMIN_EMAIL` | `{}` |", Value: config.DefaultAdminEmail},
		value{Phrase: "| `ADMIN_PASSWORD` | | {} to ", Value: config.MinPasswordLength},
		value{Phrase: " to {} characters. While there is no admin", Value: config.MaxPasswordLength},
		value{Phrase: "| `SESSION_LIFETIME` | `{}` |", Value: days(config.DefaultSessionLifetime)},
		value{Phrase: "REST API](api.md), at least {} characters", Value: config.MinAPITokenLength},
		value{Phrase: "| `RATE_LIMIT_EVENTS_PER_MINUTE` | `{}` |", Value: config.DefaultRateLimitEventsPerMinute},
		value{Phrase: "| `RATE_LIMIT_LOGS_PER_MINUTE` | `{}` |", Value: config.DefaultRateLimitLogsPerMinute},
		value{Phrase: "until it is stored, up to {}% of `MAX_DATABASE_SIZE`", Value: config.InboxPercent},
		value{Phrase: "Keep it at least `{}`, the most logs", Value: config.SDKLogBatch},
		value{Phrase: "Log levels to store: {}, or `all`/`none`", Value: "`" + strings.Join(config.HighestFirst(config.LogLevels), "`, `") + "`"},
		value{Phrase: "**Logs:** the body {} KB;", Value: config.MaxLogBodyBytes >> 10},
		value{Phrase: "an attribute's value {} KB", Value: config.MaxLogAttributeBytes >> 10},
		value{Phrase: "| `SECRET_KEY` | generated | At least {} characters", Value: secret.MinKeyLength},
		value{Phrase: "| `SMTP_PORT` | `{}` |", Value: config.DefaultSMTPPort},
		value{Phrase: "| `NOTIFY_RETRY_FOR` | `{}` |", Value: days(config.DefaultNotifyRetryFor)},
		value{Phrase: "| `NOTIFY_KEEP_SENT` | `{}` |", Value: days(config.DefaultNotifyKeepSent)},
		value{Phrase: "| `NOTIFY_KEEP_FAILED` | `{}` |", Value: days(config.DefaultNotifyKeepFailed)},
		value{Phrase: "keeps at least {}% of the queue", Value: config.MinInboxSharePercent},
		value{Phrase: "of the queue, and at least {} MB", Value: ingest.MaxBodyBytes >> 20},
		value{Phrase: "| `MAX_DATABASE_SIZE` | `{}` |", Value: size(config.DefaultMaxDatabaseBytes)},
		value{Phrase: "reaches {}% of it, the least useful", Value: config.CleanupStartPercent},
		value{Phrase: "an email, up to {} characters", Value: config.MaxEmail},
		value{Phrase: "A request from an app: up to {} MB", Value: ingest.MaxBodyBytes >> 20},
		value{Phrase: "at most {} attributes;", Value: config.MaxLogAttributes},
		value{Phrase: "**Events:** every text {} KB", Value: config.MaxEventStringBytes >> 10},
		value{Phrase: "An event still over {} MB loses", Value: config.MaxEventBytes >> 20},
		value{Phrase: "breadcrumbs beyond the newest {}, extra data", Value: config.TruncatedBreadcrumbs},
		value{Phrase: "Port {} uses TLS, others", Value: config.ImplicitTLSPort},
		value{Phrase: "deleted until it is at {}%. Give", Value: config.CleanupStopPercent},
		value{Phrase: "Give the disk {} times this", Value: diskFactor},
		value{Phrase: "At least `{}`; `0` turns", Value: size(config.MinDatabaseBytes)},
		value{Phrase: "keeping the newest {} of each issue", Value: config.OldEventsPerIssue},
		value{Phrase: "The first {} always run", Value: numberWord(store.CleanupAlwaysSteps)},
		value{Phrase: "only until the data is back at {}%.", Value: config.CleanupStopPercent},
		value{Phrase: "Every issue down to its newest {} events", Value: config.OldEventsPerIssue},
		value{Phrase: "keeps its newest {} copies", Value: config.LogsPerMessage},
		value{Phrase: "An issue stores at most {} events a minute", Value: config.EventsPerIssuePerMinute},
	)

	steps := regexp.MustCompile(`(?m)^\d+\. `).FindAllString(retention(t), -1)

	if len(steps) != len(database.CleanupSteps()) {
		t.Errorf("docs/configuration.md lists %d cleanup steps, the code has %d", len(steps), len(database.CleanupSteps()))
	}

	uid := captured(t, "Dockerfile", `(?m)^USER (\d+):`)
	grace := captured(t, "compose.yml", `stop_grace_period: (\d+)s`)

	fixtures.DocValues(t, "docs/deployment.md",
		value{Phrase: "an unprivileged user (uid {})", Value: uid},
		value{Phrase: "`sudo chown {}:", Value: uid},
		value{Phrase: "`compose.yml` gives it {} s", Value: grace},
		value{Phrase: "add `--stop-timeout {}`", Value: grace},
		value{Phrase: "ADMIN_PASSWORD=<at least {} characters>", Value: config.MinPasswordLength},
		value{Phrase: "open the UI and log in as `{}`.", Value: config.DefaultAdminEmail},
		value{Phrase: "request bodies up to {} MB", Value: ingest.MaxBodyBytes >> 20},
		value{Phrase: "max_size {}MB", Value: ingest.MaxBodyBytes >> 20},
		value{Phrase: "client_max_body_size {}m;", Value: ingest.MaxBodyBytes >> 20},
		value{Phrase: "**admin** ({})", Value: store.RoleSummaries[store.RoleAdmin]},
		value{Phrase: "**user** ({})", Value: store.RoleSummaries[store.RoleUser]},
		value{Phrase: "The server uses about {} MB of memory", Value: config.DefaultMemoryLimit >> 20},
		value{Phrase: "| Disk | {} × `MAX_DATABASE_SIZE`", Value: diskFactor},
		value{Phrase: "the authorized redirect URI `<PUBLIC_URL>{}`", Value: config.GoogleRedirectPath},
		value{Phrase: "(for example `https://errors.example.com{}`)", Value: config.GoogleRedirectPath},
	)

	fixtures.DocValues(t, "docs/sdk.md",
		value{Phrase: "(`{}` for a new one)", Value: notifyLevels},
		value{Phrase: "its issue stores {} events a minute", Value: config.EventsPerIssuePerMinute},
	)

	fixtures.DocValues(t, "docs/ui.md",
		value{Phrase: "When it reaches {}% of it, the project's", Value: config.CleanupStartPercent},
		value{Phrase: "down to {}% of its share, so", Value: config.CleanupStopPercent},
		value{Phrase: "Gets a {}-digit code first", Value: config.VerificationCodeDigits},
		value{Phrase: "a new connection sends {}. Events", Value: strings.Join(config.NotifyLevels, " and ")},
		value{Phrase: "the newest {} sign-ins", Value: store.SignInsKept},
		value{Phrase: "the newest {} refused attempts", Value: store.SignInsKept},
	)

	fixtures.DocValues(t, "README.md",
		value{Phrase: "ADMIN_PASSWORD='<at least {} characters>'", Value: config.MinPasswordLength},
		value{Phrase: "log in as `{}` with that password", Value: config.DefaultAdminEmail},
	)
	fixtures.DocValues(t, "skills/exceptic/SKILL.md",
		value{Phrase: "`ADMIN_PASSWORD` ({}+ characters)", Value: config.MinPasswordLength},
		value{Phrase: "Tell the user to log in as `{}`", Value: config.DefaultAdminEmail},
		value{Phrase: "sends only `{}`, so", Value: notifyLevels},
	)
	fixtures.DocValues(t, ".env.example", value{Phrase: "redirect URI <PUBLIC_URL>{} (needs", Value: config.GoogleRedirectPath})
}

// captured is what pattern captures in a file of the repository: a value
// the documents copy from it.
func captured(t *testing.T, file, pattern string) string {
	content, err := os.ReadFile(filepath.Join(fixtures.Root(), file))

	if err != nil {
		t.Fatal(err)
	}

	match := regexp.MustCompile(pattern).FindSubmatch(content)

	if match == nil {
		t.Fatalf("%s: no %s", file, pattern)
	}

	return string(match[1])
}

// retention is the Retention section of configuration.md.
func retention(t *testing.T) string {
	content, err := os.ReadFile("configuration.md")

	if err != nil {
		t.Fatal(err)
	}

	_, section, ok := strings.Cut(string(content), "\n## Retention\n")

	if !ok {
		t.Fatal("docs/configuration.md has no Retention section")
	}

	section, _, _ = strings.Cut(section, "\n## ")

	return section
}

func days(duration time.Duration) string {
	if duration%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", duration/(24*time.Hour))
	}

	return duration.String()
}

// size is a byte count as MAX_DATABASE_SIZE is written.
func size(bytes int64) string {
	if bytes%(1<<30) == 0 {
		return fmt.Sprintf("%dGB", bytes>>30)
	}

	return fmt.Sprintf("%dMB", bytes>>20)
}

func numberWord(number int) string {
	return []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}[number]
}
