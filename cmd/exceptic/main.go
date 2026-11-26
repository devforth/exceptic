package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/devforth/exceptic/internal/app"
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/logging"
	"github.com/devforth/exceptic/internal/platform/secret"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/web"
)

// version is set at build time: -ldflags "-X main.version=..."
var version = "dev"

const usage = `usage: exceptic [command]

Without a command it runs the server. Commands:
  healthcheck               exit 0 when the server answers /health (Docker HEALTHCHECK)
  reset-password [email]    print a temporary password (the superadmin by default); the server must be stopped
  backup [dir]              copy the database (default DATA_DIR/backups/<time>)
  version                   print the version`

func main() {
	os.Exit(execute(os.Args[1:]))
}

// execute runs the command in args and returns the exit code.
func execute(args []string) int {
	if len(args) == 0 {
		return exitCode(run())
	}

	switch args[0] {
	case "healthcheck":
		// The image is FROM scratch: there is no curl or wget inside.
		return healthcheck()
	case "reset-password":
		return exitCode(resetPassword(args[1:]))
	case "backup":
		return exitCode(backup(args[1:]))
	case "version":
		fmt.Println(version)

		return 0
	default:
		fmt.Fprintln(os.Stderr, usage)

		return 2
	}
}

func exitCode(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)

		return 1
	}

	return 0
}

func run() error {
	settings, err := config.Load()

	if err != nil {
		return err
	}

	config.LimitMemory()

	logger := logging.New(os.Stderr, settings.LogFormat, settings.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	// A second signal during the shutdown ends the process at once.
	context.AfterFunc(ctx, stop)

	db, err := database.Open(settings.DataDir)

	if err != nil {
		return err
	}

	defer func() {
		// Closing flushes the store; a failure here is worth knowing about.
		if err := db.Close(); err != nil {
			logger.Error("close database", "err", err)
		}
	}()

	if settings.APIToken == "" {
		logger.Warn("API_TOKEN is not set, the REST API is closed")
	}

	if len(settings.AllowedIPs) == 0 {
		logger.Warn("ALLOWED_IPS is not set, the API and UI are reachable from any address")
	}

	if os.Getenv("ADMIN_USERNAME") != "" {
		logger.Warn("ADMIN_USERNAME is renamed: set ADMIN_EMAIL (read instead while ADMIN_EMAIL is not set)")
	}

	if os.Getenv("ALLOWED_ORIGINS") != "" {
		logger.Warn("ALLOWED_ORIGINS is no longer used: set the allowed domains of each project in its settings")
	}

	if os.Getenv("RATE_LIMIT_PER_MINUTE") != "" {
		logger.Warn("RATE_LIMIT_PER_MINUTE is no longer used: set RATE_LIMIT_EVENTS_PER_MINUTE and RATE_LIMIT_LOGS_PER_MINUTE")
	}

	if limit := settings.RateLimitLogsPerMinute; limit > 0 && limit < config.SDKLogBatch {
		logger.Warn("RATE_LIMIT_LOGS_PER_MINUTE is below the logs an SDK sends in one batch: a batch over it is dropped whole",
			"limit", limit, "batch", config.SDKLogBatch)
	}

	// Chat tokens are stored encrypted: without SECRET_KEY the key is the one
	// generated into the data directory.
	if settings.Notifications.SecretKey == "" {
		key, created, err := secret.KeyFile(settings.DataDir)

		if err != nil {
			return fmt.Errorf("secret key: %w", err)
		}

		if created {
			logger.Info("secret key generated: set SECRET_KEY to keep it outside the data directory", "file", filepath.Join(settings.DataDir, secret.KeyFileName))
		}

		settings.Notifications.SecretKey = key
	}

	factory, err := notificationFactory(logger, settings.AppName, settings.Notifications)

	if err != nil {
		return err
	}

	// The web app is embedded at build time; a build without Node has none.
	spa, err := web.New()

	if errors.Is(err, web.ErrNotBuilt) {
		logger.Warn("web app not built: no web UI is served", "build", "make web")
	} else if err != nil {
		return err
	}

	application, err := app.New(ctx, app.Options{Settings: settings, Logger: logger, Store: db, Factory: factory, Version: version, Web: spa})

	if err != nil {
		return err
	}

	if len(application.Registry.List()) == 0 {
		logger.Warn("no projects yet, events will be rejected; add one on the Projects page or set PROJECTS=id:name:public_key,...")
	}

	logRetention(logger, settings.Retention)

	// Bind before logging "started" so a busy port fails here, not after a false start message.
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", settings.Port))

	if err != nil {
		return err
	}

	logger.Info("server started", logging.Block, "url", serverURL(settings, listener.Addr()), "version", version, "data_dir", settings.DataDir, "projects", len(application.Registry.List()))

	return application.Run(ctx, listener)
}

// serverURL is where to open the server: PUBLIC_URL, or this machine at
// the port it listens on.
func serverURL(settings *config.Config, address net.Addr) string {
	if settings.Notifications.PublicURL != "" {
		return settings.Notifications.PublicURL
	}

	if tcp, ok := address.(*net.TCPAddr); ok {
		return "http://localhost:" + strconv.Itoa(tcp.Port)
	}

	return address.String()
}

// logRetention says at startup how the database is kept small, so a
// surprising deletion can be traced to the setting.
func logRetention(logger *slog.Logger, settings config.Retention) {
	if settings.MaxDatabaseBytes == 0 {
		logger.Info("retention", "max_database_size", "0 (nothing is deleted)")

		return
	}

	logger.Info("retention", logging.Block, "max_database_size", config.FormatSize(settings.MaxDatabaseBytes),
		"cleanup", fmt.Sprintf("from %d%% to %d%% of the data held, or when the files stay over %d%%", config.CleanupStartPercent, config.CleanupStopPercent, config.CleanupFilesPercent))
}

// notificationFactory builds channels for the connections set up in the UI.
// It logs which connection types can be created with the current settings.
func notificationFactory(logger *slog.Logger, appName string, settings config.Notifications) (notify.Factory, error) {
	factory := notify.Factory{AppName: appName, SMTP: settings.SMTP, PublicURL: settings.PublicURL, Unsubscribe: notify.NewUnsubscriber(settings.SecretKey)}

	if settings.SecretKey != "" {
		box, err := secret.New(settings.SecretKey)

		if err != nil {
			return factory, fmt.Errorf("SECRET_KEY: %w", err)
		}

		factory.Box = box
	}

	// One field per type: "on", or what is missing.
	channels := []any{logging.Block}

	for _, channelType := range config.AllChannelTypes {
		state := "on"

		if ok, reason := settings.ChannelAvailable(channelType); !ok {
			state = "off: " + reason
		}

		channels = append(channels, channelType, state)
	}

	logger.Info("notification channels", channels...)

	if settings.PublicURL == "" {
		logger.Warn("PUBLIC_URL is not set, notifications will not link to the issue")
	}

	return factory, nil
}

func healthcheck() int {
	port := os.Getenv("PORT")

	if port == "" {
		port = strconv.Itoa(config.DefaultPort)
	}

	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + port + "/health")

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	defer func() { _ = response.Body.Close() }() // read-only, nothing to lose on close

	if response.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy:", response.Status)
		return 1
	}

	return 0
}

// resetPassword gives a user (the superadmin when no email is given) a
// temporary password and prints it: they choose their own at the next
// login. Their sessions end; connected agents keep working.
func resetPassword(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: exceptic reset-password [email]")
	}

	settings, err := config.Load()

	if err != nil {
		return err
	}

	ctx := context.Background()
	logger := logging.New(os.Stderr, settings.LogFormat, settings.LogLevel)

	if _, err := os.Stat(database.Location(settings.DataDir)); err != nil {
		return fmt.Errorf("no database in %s: %w", settings.DataDir, err)
	}

	db, err := database.Open(settings.DataDir)

	// The running server holds the database: the command runs while it is stopped.
	if errors.Is(err, database.ErrLocked) {
		return fmt.Errorf("stop the server first, the database is open there (%w)", err)
	}

	if err != nil {
		return err
	}

	defer func() { _ = db.Close() }()

	email := ""

	if len(args) == 1 {
		email = args[0]
	} else {
		superadmin, err := db.Superadmin(ctx)

		if err != nil {
			return fmt.Errorf("no superadmin yet: set ADMIN_PASSWORD and start the server: %w", err)
		}

		email = superadmin.Email
	}

	users := services.NewUsers(logger, db)
	password, err := users.SetPasswordByOperator(ctx, email)

	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no user %q", email)
	}

	if err != nil {
		return err
	}

	fmt.Printf("Temporary password of %s: %s\nLog in with it and choose a new password.\n", email, password)

	return nil
}
