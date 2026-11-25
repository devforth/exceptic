package services

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Connections: the notification connections, what tells them apart and
// names them, and their delivery health.
type Connections struct {
	logger        *slog.Logger
	store         store.Store
	factory       notify.Factory
	notifier      *notify.Notifier // nil: no outbox worker to wake
	projects      config.ProjectList
	notifications config.Notifications
	now           func() time.Time
}

func newConnections(logger *slog.Logger, database store.Store, factory notify.Factory, projects config.ProjectList, settings *config.Config) *Connections {
	return &Connections{logger: logger, store: database, factory: factory, projects: projects, notifications: settings.Notifications, now: time.Now}
}

// New is what a new connection of channelType starts with
// (store.NewConnection): every project, or the projects of a limited access.
func (service *Connections) New(channelType string, access store.Access) store.Connection {
	connection := store.NewConnection(channelType)

	if !access.All() {
		connection.AllProjects, connection.ProjectIDs = false, nil

		for _, project := range access.Visible(service.projects.List()) {
			connection.ProjectIDs = append(connection.ProjectIDs, project.ID)
		}
	}

	return connection
}

// List returns the connections within access (Visible).
func (service *Connections) List(ctx context.Context, access store.Access) ([]store.Connection, error) {
	connections, err := service.store.ListConnections(ctx)

	if err != nil || access.All() {
		return connections, err
	}

	return slices.DeleteFunc(connections, func(connection store.Connection) bool { return !Visible(access, connection) }), nil
}

// Visible: access sees the connection, which sends only for projects it
// sees. A connection for every project, or for none (its projects were
// deleted), is seen only with every project.
func Visible(access store.Access, connection store.Connection) bool {
	return access.All() || (!connection.AllProjects && len(connection.ProjectIDs) > 0 && access.Covers(store.OnlyProjects(connection.ProjectIDs...)))
}

// DeliveryHealth sums up the delivery of each of the connections (read
// through an access), by connection id.
func (service *Connections) DeliveryHealth(ctx context.Context, connections ...store.Connection) (map[string]store.DeliveryHealth, error) {
	all, err := service.store.DeliveryHealthByChannel(ctx)
	health := map[string]store.DeliveryHealth{}

	for _, connection := range connections {
		if summary, ok := all[connection.ID]; ok {
			health[connection.ID] = summary
		}
	}

	return health, err
}

// Get reads a connection; store.ErrNotFound when it does not exist,
// ErrHidden outside access.
func (service *Connections) Get(ctx context.Context, access store.Access, id string) (store.Connection, error) {
	connection, err := service.store.GetConnection(ctx, id)

	switch {
	case errors.Is(err, store.ErrNotFound):
		return store.Connection{}, Missing("connection", id)
	case err == nil && !Visible(access, connection):
		return store.Connection{}, Hidden("connection", id)
	}

	return connection, err
}

// create saves a new connection and returns its id. chatSecret is the
// encrypted secret of a chat (empty for email): the chat's destination is
// created first.
func (service *Connections) create(ctx context.Context, connection store.Connection, chatSecret string) (string, error) {
	if chatSecret != "" {
		// Chats need no confirmation: whoever has the token owns the chat.
		now := service.now()
		destinationID, err := service.store.CreateDestination(ctx, store.Destination{Type: connection.Type, Secret: chatSecret, Verified: true, VerifiedAt: now}, now)

		if err != nil {
			return "", err
		}

		connection.DestinationID = destinationID
	}

	return service.store.CreateConnection(ctx, connection, service.now())
}

// update saves a connection. newChatSecret is the encrypted chat secret read
// from the form (empty for email); the destination gets it when it differs
// from the stored one.
func (service *Connections) update(ctx context.Context, connection store.Connection, newChatSecret string) error {
	if newChatSecret != "" && newChatSecret != connection.Secret {
		if err := service.store.UpdateDestinationSecret(ctx, connection.DestinationID, newChatSecret); err != nil {
			return err
		}
	}

	return service.saved(ctx, connection)
}

// SetActive turns a connection on or off; an unconfirmed address cannot be
// turned on (store.ErrNotVerified).
func (service *Connections) SetActive(ctx context.Context, connection store.Connection, active bool) error {
	if active && !connection.Verified {
		return store.ErrNotVerified
	}

	connection.Active = active

	return service.saved(ctx, connection)
}

// Unsubscribe pauses every connection sending to an email destination,
// for its link in an email (notify.Unsubscriber, checked by the caller):
// they stay listed, paused, so an admin can resume them. It returns the
// address and how many it paused; an unknown destination is store.ErrNotFound.
func (service *Connections) Unsubscribe(ctx context.Context, destinationID string) (address string, paused int, err error) {
	connections, err := service.store.ListConnections(ctx)

	if err != nil {
		return "", 0, err
	}

	found := false

	for _, connection := range connections {
		if connection.DestinationID != destinationID || connection.Type != config.ChannelEmail {
			continue
		}

		found, address = true, connection.Address

		if !connection.Active {
			continue
		}

		if err := service.SetActive(ctx, connection, false); err != nil {
			return address, paused, err
		}

		paused++
	}

	if !found {
		return "", 0, store.ErrNotFound
	}

	return address, paused, nil
}

// RetryNow makes the waiting messages of a connection within access due now.
func (service *Connections) RetryNow(ctx context.Context, access store.Access, id string) error {
	if _, err := service.Get(ctx, access, id); err != nil {
		return err
	}

	return service.retryNow(ctx, id)
}

func (service *Connections) retryNow(ctx context.Context, id string) error {
	if err := service.store.RetryOutboxNow(ctx, id, service.now()); err != nil {
		return err
	}

	// A rate limit's pause ends too: the fix may have been a new URL.
	if service.notifier != nil {
		service.notifier.RetryNow(id)
	}

	return nil
}

// saved stores a changed connection and retries its waiting messages now:
// a save is usually the fix for why they failed.
func (service *Connections) saved(ctx context.Context, connection store.Connection) error {
	if err := service.store.UpdateConnection(ctx, connection); err != nil {
		return err
	}

	if !connection.Active {
		return nil
	}

	return service.retryNow(ctx, connection.ID)
}

// Delete removes a connection within access (outside it nothing, as for
// one that does not exist); one already gone (deleted from another tab) is not an error. Its
// destination stays, so a confirmed address stays confirmed.
func (service *Connections) Delete(ctx context.Context, access store.Access, id string) error {
	connection, err := service.store.GetConnection(ctx, id)

	switch {
	case errors.Is(err, store.ErrNotFound) || (err == nil && !Visible(access, connection)):
		return nil // as one already gone
	case err != nil:
		return err
	}

	if err := service.store.DeleteConnection(ctx, id); err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}

	if service.notifier != nil {
		service.notifier.ConnectionDeleted(id)
	}

	return nil
}

// defaultName names a connection saved without a name: by the address's
// local part, the Telegram chat's title or "Slack"; secret is encrypted.
func (service *Connections) defaultName(ctx context.Context, connection store.Connection, secret string) string {
	switch connection.Type {
	case config.ChannelEmail:
		return textutil.TruncateRunes(textutil.EmailLocalPart(connection.Address), config.MaxConnectionName)
	case config.ChannelTelegram:
		var tokens notify.TelegramSecret

		if service.factory.DecryptSecret(secret, &tokens) != nil {
			return "Telegram"
		}

		lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)

		defer cancel()

		title, err := service.factory.TelegramChatTitle(lookupCtx, tokens)

		if err != nil || strings.TrimSpace(title) == "" {
			service.logger.Warn("telegram chat name not available", "err", err)

			return textutil.TruncateRunes("Telegram "+tokens.ChatID, config.MaxConnectionName)
		}

		return textutil.TruncateRunes(strings.TrimSpace(title), config.MaxConnectionName)
	default:
		return DefaultSlackName
	}
}

// DefaultSlackName names a Slack connection saved without a name.
const DefaultSlackName = "Slack"

// Types are the types a new connection may have, in config.AllChannelTypes
// order: whether each can be used now (what is missing if not), whether it
// links to the issue by default and what an empty name becomes.
func (service *Connections) Types() []ChannelType {
	var types []ChannelType

	for _, channelType := range config.AllChannelTypes {
		available, reason := service.notifications.ChannelAvailable(channelType)
		types = append(types, ChannelType{
			Type: channelType, Available: available, Reason: reason,
			IncludeIssueLink: !store.NewConnection(channelType).HideIssueLink,
			NamePlaceholder:  namePlaceholders[channelType],
		})
	}

	return types
}

// namePlaceholders: what an empty name becomes, as the form says it; an
// email connection takes the part of the address before "@" ("team" before
// one is entered).
var namePlaceholders = map[string]string{
	config.ChannelEmail:    "team",
	config.ChannelTelegram: "the name of the chat, from Telegram",
	config.ChannelSlack:    DefaultSlackName,
}

// IssueLinksAvailable: messages can link to the issue page (PUBLIC_URL is set).
func (service *Connections) IssueLinksAvailable() bool {
	return service.notifications.PublicURL != ""
}

// Destination says where a connection sends, never its token: the address,
// the chat, or the Slack bot's channel.
func (service *Connections) Destination(connection store.Connection) string {
	if connection.Type == config.ChannelEmail {
		return connection.Address
	}

	switch chat := service.Chat(connection); {
	case !chat.Readable:
		return "token cannot be decrypted (SECRET_KEY changed?)"
	case connection.Type == config.ChannelTelegram:
		return "chat " + chat.Telegram.ChatID
	case chat.Slack.BotToken != "":
		return "bot in " + chat.Slack.Channel + ", prompt in thread"
	default:
		return "incoming webhook"
	}
}

// Chat is what the secret of a chat connection holds; Readable is false when
// it cannot be decrypted (SECRET_KEY changed).
type Chat struct {
	Telegram notify.TelegramSecret
	Slack    notify.SlackSecret
	Readable bool
}

// Chat decrypts the secret of a Telegram or Slack connection.
func (service *Connections) Chat(connection store.Connection) Chat {
	var chat Chat

	switch connection.Type {
	case config.ChannelTelegram:
		chat.Readable = service.factory.DecryptSecret(connection.Secret, &chat.Telegram) == nil
	case config.ChannelSlack:
		chat.Readable = service.factory.DecryptSecret(connection.Secret, &chat.Slack) == nil
	}

	return chat
}

// destinationKey is what makes two connections send to the same place,
// from the decrypted secret of a chat (its ciphertext differs every time).
func (service *Connections) destinationKey(connection store.Connection) string {
	if connection.Type == config.ChannelEmail {
		return strings.ToLower(connection.Address)
	}

	switch chat := service.Chat(connection); {
	case !chat.Readable:
		return ""
	case connection.Type == config.ChannelTelegram:
		return chat.Telegram.ChatID
	default:
		return chat.Slack.Key()
	}
}

// findDuplicate returns the connection other than exceptID that already
// sends to key (see destinationKey), or nil; one outside access is not
// named (an empty Duplicate).
func (service *Connections) findDuplicate(ctx context.Context, access store.Access, channelType, key string, exceptID string) (*Duplicate, error) {
	if key == "" {
		return nil, nil
	}

	connections, err := service.store.ListConnections(ctx)

	if err != nil {
		return nil, err
	}

	for _, connection := range connections {
		if connection.ID != exceptID && connection.Type == channelType && service.destinationKey(connection) == key {
			if !Visible(access, connection) {
				return &Duplicate{}, nil
			}

			return &Duplicate{ID: connection.ID, Name: connection.Name}, nil
		}
	}

	return nil, nil
}
