package contracts

import (
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

type TypeChoice struct {
	Value, Label, Reason string
	Available, Selected  bool
}

// NamePlaceholder is what an empty name becomes for this type (app.js shows it when the type is chosen).
func (choice TypeChoice) NamePlaceholder() string { return namePlaceholder(choice.Value, "") }

// IssueLink: a new connection of this type includes the issue link (app.js sets it when the type is chosen).
func (choice TypeChoice) IssueLink() bool {
	return !store.NewConnection(choice.Value, config.Recording{}).HideIssueLink
}

type Choice struct {
	Value   string
	Label   string
	Checked bool
}

// ConnectionView is what templates/notifications/connection.html shows: the form of a new
// or an existing connection, with errors after a failed save.
type ConnectionView struct {
	ID string
	// Waiting is how many messages wait for a retry after a failed attempt
	// (the "Retry now" button is shown for them).
	Waiting      int
	New          bool
	Name         string
	Type         string
	TypeLabel    string
	Email        services.EmailStep
	ChatID       string // shown; the bot token never is
	SlackChannel string // the channel of a Slack bot
	// The stored secrets, masked (helpers.MaskSecret) so the form can say
	// what is saved without showing it; empty when none is stored. An empty
	// field keeps the stored value.
	SavedWebhook       string
	SavedSlackToken    string
	SavedTelegramToken string
	AllProjects        bool
	Active             bool
	HideIssueLink      bool
	Verified           bool
	Types              []TypeChoice
	Projects           []Choice
	Levels             []Choice
	Errors             map[string]string
	Duplicate          *services.Duplicate // a chat already used by another connection
	Error              string              // a failure that is not about one field (sending, saving)
	Notice             string
}

// NamePlaceholder is what an empty name becomes for the shown type (app.js
// updates it as the type or the address changes).
func (view ConnectionView) NamePlaceholder() string {
	return namePlaceholder(view.Type, view.Email.Address)
}

func namePlaceholder(channelType, address string) string {
	switch channelType {
	case config.ChannelTelegram:
		return "the name of the chat, from Telegram"
	case config.ChannelSlack:
		return services.DefaultSlackName
	}

	if local := textutil.EmailLocalPart(address); local != "" {
		return local
	}

	return "team"
}

// IsEmail, IsTelegram and IsSlack: the type of the connection.
func (view ConnectionView) IsEmail() bool { return view.Type == config.ChannelEmail }

func (view ConnectionView) IsTelegram() bool { return view.Type == config.ChannelTelegram }

func (view ConnectionView) IsSlack() bool { return view.Type == config.ChannelSlack }

// ChosenProjects and ChosenLevels are the checked choices, shown while their section is closed.
func (view ConnectionView) ChosenProjects() []Choice { return chosen(view.Projects) }

func (view ConnectionView) ChosenLevels() []Choice { return chosen(view.Levels) }

func chosen(choices []Choice) []Choice {
	var result []Choice

	for _, choice := range choices {
		if choice.Checked {
			result = append(result, choice)
		}
	}

	return result
}

// RestLocked tells the template to disable everything after the address
// until the address of an email connection is confirmed.
func (view ConnectionView) RestLocked() bool {
	return view.IsEmail() && !view.Email.Verified()
}

type ConnectionRow struct {
	ID          string
	Name        string
	TypeLabel   string
	Destination string
	AllProjects bool
	Projects    []string // names, shown with their colors
	Levels      string
	Status      string
	StatusClass string // active, inactive or awaiting
	Active      bool
	Verified    bool
	Delivery    store.DeliveryHealth
}

type ConnectionsView struct {
	Rows   []ConnectionRow
	Notice string
}
