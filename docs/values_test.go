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
		value{Phrase: "The id is from 1 to {},", Value: config.MaxProjectID},
		value{Phrase: "the key is {}–", Value: config.MinPublicKey},
		value{Phrase: "–{} letters or digits", Value: config.MaxPublicKey},
		value{Phrase: "| `ADMIN_EMAIL` | `{}` |", Value: config.DefaultAdminEmail},
		value{Phrase: "or up to {} letters, digits", Value: config.MaxEmail},
		value{Phrase: "| `ADMIN_PASSWORD` | | {} to ", Value: config.MinPasswordLength},
		value{Phrase: " to {} characters. Needed for the first start", Value: config.MaxPasswordLength},
		value{Phrase: "| `SESSION_LIFETIME` | `{}` |", Value: days(config.DefaultSessionLifetime)},
		value{Phrase: "REST API](api.md), at least {} characters", Value: config.MinAPITokenLength},
		value{Phrase: "| `RATE_LIMIT_EVENTS_PER_MINUTE` | `{}` |", Value: config.DefaultRateLimitEventsPerMinute},
		value{Phrase: "| `RATE_LIMIT_LOGS_PER_MINUTE` | `{}` |", Value: config.DefaultRateLimitLogsPerMinute},
		value{Phrase: "Keep it at least `{}`, the most logs", Value: config.SDKLogBatch},
		value{Phrase: "Log levels to store: {}, or `all`/`none`", Value: "`" + strings.Join(config.HighestFirst(config.LogLevels), "`, `") + "`"},
		value{Phrase: "An envelope may have {} MB, before", Value: ingest.MaxBodyBytes >> 20},
		value{Phrase: "**Logs:** the body {} KB;", Value: config.MaxLogBodyBytes >> 10},
		value{Phrase: "at most {} attributes:", Value: config.MaxLogAttributes},
		value{Phrase: "an attribute's value {} KB", Value: config.MaxLogAttributeBytes >> 10},
		value{Phrase: "**Events:** every string value {} KB", Value: config.MaxEventStringBytes >> 10},
		value{Phrase: "An event still over {} MB then loses", Value: config.MaxEventBytes >> 20},
		value{Phrase: "breadcrumbs beyond the newest {}, `extra`", Value: config.TruncatedBreadcrumbs},
		value{Phrase: "an event still over {} MB after them", Value: config.MaxEventBytes >> 20},
		value{Phrase: "| `SECRET_KEY` | generated | At least {} characters", Value: secret.MinKeyLength},
		value{Phrase: "| `SMTP_PORT` | `{}` |", Value: config.DefaultSMTPPort},
		value{Phrase: "Port {} uses TLS from the start", Value: config.ImplicitTLSPort},
		value{Phrase: "| `NOTIFY_RETRY_FOR` | `{}` |", Value: days(config.DefaultNotifyRetryFor)},
		value{Phrase: "| `NOTIFY_KEEP_SENT` | `{}` |", Value: days(config.DefaultNotifyKeepSent)},
		value{Phrase: "| `NOTIFY_KEEP_FAILED` | `{}` |", Value: days(config.DefaultNotifyKeepFailed)},
		value{Phrase: "as they turn a month old, every {} minutes", Value: minutes(config.CleanupInterval)},
		value{Phrase: "| `MAX_DATABASE_SIZE` | `{}` |", Value: size(config.DefaultMaxDatabaseBytes)},
		value{Phrase: "reaches {}% of it, the least useful", Value: config.CleanupStartPercent},
		value{Phrase: "until it holds {}%. The files", Value: config.CleanupStopPercent},
		value{Phrase: "give the disk at least {} times this", Value: diskFactor},
		value{Phrase: "at least `{}`; `0` turns", Value: size(config.MinDatabaseBytes)},
		value{Phrase: "may take {}% of it (of", Value: config.InboxPercent},
		value{Phrase: "% of it (of `{}` when it is off)", Value: size(config.DefaultMaxDatabaseBytes)},
		value{Phrase: "checked at startup, every {} minutes and after", Value: minutes(config.CleanupInterval)},
		value{Phrase: "after every {} MB written", Value: config.CleanupWritten >> 20},
		value{Phrase: "At startup and every {} minutes, halfway", Value: minutes(config.CleanupInterval)},
		value{Phrase: "once they are more than {}% of the data, so about", Value: config.StalePercent},
		value{Phrase: "queue: up to {}% of the limit under a peak", Value: config.InboxPercent},
		value{Phrase: "the files take more than {}% of the limit", Value: config.CleanupFilesPercent},
		value{Phrase: "with the data under {}%, until", Value: config.CleanupStartPercent},
		value{Phrase: "until the data is at {}% and the files", Value: config.CleanupStopPercent},
		value{Phrase: "once deleted records are more than {}% of the data (below", Value: config.StalePercent},
		value{Phrase: "tried again on the next passes of its job, {} attempts in all", Value: store.CompactionAttempts},
		value{Phrase: "Events older than a month beyond the newest {} of their issue", Value: config.OldEventsPerIssue},
		value{Phrase: "These {} run in full", Value: numberWord(store.CleanupAlwaysSteps)},
		value{Phrase: "only while the data is still above {}% (or", Value: config.CleanupStopPercent},
		value{Phrase: "Every issue down to its newest {} events", Value: config.OldEventsPerIssue},
		value{Phrase: "capped at {} a minute per issue", Value: config.EventsPerIssuePerMinute},
		value{Phrase: "down to {}% of its share. A project's", Value: config.CleanupStopPercent},
		value{Phrase: "A project's run starts at {}% of its share", Value: config.CleanupStartPercent},
		value{Phrase: "if the database is still at {}% of its limit", Value: config.CleanupStartPercent},
		value{Phrase: "records are more than {}% of the data: then", Value: config.StalePercent},
		value{Phrase: "and the last {} runs with their steps", Value: store.CleanupRunsKept},
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
		value{Phrase: "may hold {} MB together", Value: ingest.MaxReadingBytes >> 20},
		value{Phrase: "half of it or {} MB", Value: ingest.MaxProjectReadingBytes >> 20},
		value{Phrase: "a body must arrive within {} seconds", Value: seconds(ingest.BodyReadTimeout)},
		value{Phrase: "**superadmin** ({})", Value: store.RoleSummaries[store.RoleSuperadmin]},
		value{Phrase: "**admin** ({})", Value: store.RoleSummaries[store.RoleAdmin]},
		value{Phrase: "**user** ({})", Value: store.RoleSummaries[store.RoleUser]},
		value{Phrase: "About {} MB at the peak (the default `GOMEMLIMIT`)", Value: config.DefaultMemoryLimit >> 20},
		value{Phrase: "| Disk space | At least {} ×", Value: diskFactor},
		value{Phrase: "| Connections | At most {} at once", Value: fixtures.Thousands(config.MaxConnections)},
		value{Phrase: "Request headers are limited to {} KiB", Value: config.MaxHeaderBytes >> 10},
		value{Phrase: "the authorized redirect URI `<PUBLIC_URL>{}`", Value: config.GoogleRedirectPath},
		value{Phrase: "(for example `https://errors.example.com{}`)", Value: config.GoogleRedirectPath},
	)

	fixtures.DocValues(t, "docs/sdk.md",
		value{Phrase: "(`{}` for a new one)", Value: notifyLevels},
		value{Phrase: "its issue stores {} events a minute", Value: config.EventsPerIssuePerMinute},
		value{Phrase: "(up to {} logs in JavaScript)", Value: config.SDKLogBatch},
		value{Phrase: "is compacted every {} minutes when ingest is quiet", Value: minutes(config.CleanupInterval)},
		value{Phrase: "(not within {} s,", Value: seconds(ingest.BodyReadTimeout)},
		value{Phrase: "Events counted over {} a minute", Value: config.EventsPerIssuePerMinute},
	)

	fixtures.DocValues(t, "docs/ui.md",
		value{Phrase: "When it reaches {}% of it, even", Value: config.CleanupStartPercent},
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

func minutes(duration time.Duration) int { return int(duration / time.Minute) }

func seconds(duration time.Duration) int { return int(duration / time.Second) }

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
