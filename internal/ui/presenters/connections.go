package presenters

import (
	"slices"
	"strconv"
	"strings"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/contracts"
	"github.com/devforth/exceptic/internal/ui/helpers"
	"github.com/devforth/exceptic/internal/ui/lists"
)

var channelLabels = map[string]string{
	config.ChannelEmail:    "Email",
	config.ChannelTelegram: "Telegram",
	config.ChannelSlack:    "Slack",
}

// Connections presents notification connections: the list and the form.
type Connections struct {
	// Chat reads the secret of a chat connection (services.Connections.Chat).
	Chat          func(connection store.Connection) services.Chat
	Notifications config.Notifications
	Recording     config.Recording // a new connection starts with its event levels
	Projects      config.ProjectList
	ProjectName   ProjectName
}

// List is the notifications page; health is the delivery health by connection id.
func (presenter Connections) List(connections []store.Connection, health map[string]store.DeliveryHealth, notice string) contracts.ConnectionsView {
	view := contracts.ConnectionsView{Notice: notice}

	for _, connection := range connections {
		row := presenter.Row(connection)
		row.Delivery = health[connection.ID]
		view.Rows = append(view.Rows, row)
	}

	return view
}

func (presenter Connections) Row(connection store.Connection) contracts.ConnectionRow {
	row := contracts.ConnectionRow{
		ID: connection.ID, Name: connection.Name, TypeLabel: channelLabels[connection.Type],
		Levels: strings.Join(config.InLevelOrder(connection.Levels, config.Levels), ", "),
	}
	row.Active, row.Verified = connection.Active, connection.Verified

	if connection.Type == config.ChannelEmail {
		row.Destination = connection.Address
	} else {
		row.Destination = chatDestination(connection.Type, presenter.Chat(connection))
	}

	row.AllProjects = connection.AllProjects

	for _, projectID := range connection.ProjectIDs {
		row.Projects = append(row.Projects, presenter.ProjectName(projectID))
	}

	switch {
	case !connection.Verified:
		row.Status, row.StatusClass = "awaiting confirmation", "awaiting"
	case connection.Active:
		row.Status, row.StatusClass = "active", "active"
	default:
		row.Status, row.StatusClass = "inactive", "inactive"
	}

	return row
}

// chatDestination says where a chat connection sends, never its token.
func chatDestination(channelType string, chat services.Chat) string {
	switch {
	case !chat.Readable:
		return "token cannot be decrypted (SECRET_KEY changed?)"
	case channelType == config.ChannelTelegram:
		return "chat " + chat.Telegram.ChatID
	case chat.Slack.BotToken != "":
		return "bot in " + chat.Slack.Channel + ", prompt in thread"
	default:
		return "incoming webhook"
	}
}

// NewConnection is what the form of a new connection starts with.
// The first type that can be used is preselected (app.js switches the
// issue link default with the type chosen in the form).
func (presenter Connections) NewConnection() store.Connection {
	for _, channelType := range config.AllChannelTypes {
		if available, _ := presenter.Notifications.ChannelAvailable(channelType); available {
			return store.NewConnection(channelType, presenter.Recording)
		}
	}

	return store.NewConnection("", presenter.Recording)
}

// Form fills the form from a connection (empty for a new one).
func (presenter Connections) Form(connection store.Connection) contracts.ConnectionView {
	view := contracts.ConnectionView{
		ID: connection.ID, New: connection.ID == "", Name: connection.Name, Type: connection.Type,
		TypeLabel: channelLabels[connection.Type], AllProjects: connection.AllProjects,
		HideIssueLink: connection.HideIssueLink,
		Active:        connection.Active, Verified: connection.Verified, Errors: map[string]string{},
		Email: services.EmailStep{State: services.EmailInput, Address: connection.Address, DestinationID: connection.DestinationID},
	}

	if connection.Type == config.ChannelEmail && !view.New {
		view.Email.Fixed = true
		view.Email.State = services.EmailUnconfirmed
	}

	if connection.Type == config.ChannelEmail && connection.Verified {
		view.Email.State = services.EmailVerified
	}

	for _, channelType := range config.AllChannelTypes {
		available, reason := presenter.Notifications.ChannelAvailable(channelType)
		view.Types = append(view.Types, contracts.TypeChoice{
			Value: channelType, Label: channelLabels[channelType], Reason: reason,
			Available: available, Selected: channelType == connection.Type,
		})
	}

	for _, project := range presenter.Projects.List() {
		view.Projects = append(view.Projects, contracts.Choice{
			Value: strconv.Itoa(project.ID), Label: project.Name, Checked: slices.Contains(connection.ProjectIDs, project.ID),
		})
	}

	for _, level := range lists.EventLevels {
		view.Levels = append(view.Levels, contracts.Choice{Value: level, Label: level, Checked: slices.Contains(connection.Levels, level)})
	}

	if chat := presenter.chat(connection); chat.Readable {
		view.ChatID, view.SavedTelegramToken = chat.Telegram.ChatID, helpers.MaskSecret(chat.Telegram.BotToken)
		view.SlackChannel = chat.Slack.Channel
		view.SavedWebhook, view.SavedSlackToken = helpers.MaskSecret(chat.Slack.WebhookURL), helpers.MaskSecret(chat.Slack.BotToken)
	}

	return view
}

// chat reads the stored secret of a chat connection; nothing for email or a new one.
func (presenter Connections) chat(connection store.Connection) services.Chat {
	if connection.Type == config.ChannelEmail || connection.Secret == "" {
		return services.Chat{}
	}

	return presenter.Chat(connection)
}

// FormAgain is the form shown again after a failed save or test: problems
// are the field errors, failure an error that is not about one field,
// existing the connection that already sends there and typedAddress the
// address field as posted.
func (presenter Connections) FormAgain(connection store.Connection, problems map[string]string, failure string, existing *services.Duplicate, typedAddress string) contracts.ConnectionView {
	view := presenter.Form(connection)
	view.Error = failure
	view.Duplicate = existing

	if problems != nil {
		view.Errors = problems
	}

	// Keep the email step where it was: a confirmed address stays confirmed.
	if connection.ID == "" && connection.Type == config.ChannelEmail {
		view.Email.Address = typedAddress

		if connection.DestinationID != "" {
			view.Email.State = services.EmailVerified
		}

		// The address is taken: back to the address field, with a link.
		if existing != nil {
			view.Email = services.EmailStep{State: services.EmailInput, Address: connection.Address, Duplicate: existing}
			view.Duplicate = nil
		}
	}

	return view
}
