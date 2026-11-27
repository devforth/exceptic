package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Notification connections, as on the Notifications page. Stored tokens are
// never returned (the web app gets them masked); an email address receives
// nothing before it is confirmed with the code mailed to it, which only its
// owner can read.
func (handler *handlers) notificationEndpoints() []Endpoint {
	return []Endpoint{
		Define(Endpoint{
			Name:        "list_connections",
			Description: "Notification connections (email, Telegram, Slack): where each sends, projects, levels, whether active and confirmed, and delivery health.",
			ReadOnly:    true, Transports: ActsAsUser,
		}, handler.listConnections),
		Define(Endpoint{
			Name:        "get_connection",
			Description: "One connection as its form shows it: project ids, chat ids, the saved tokens masked, when its address was confirmed and how many messages wait for a retry.",
			ReadOnly:    true, Transports: TransportUI,
		}, handler.getConnection),
		Define(Endpoint{
			Name:        "get_connection_defaults",
			Description: "What the form of a new connection offers: each type with what it lacks, the default type and levels, and whether messages can link to issues (PUBLIC_URL).",
			ReadOnly:    true, Transports: TransportUI,
		}, handler.getConnectionDefaults),
		Define(Endpoint{
			Name: "start_email_verification",
			Description: "First step of an email connection: mails a 6-digit code to the address, or says it is confirmed already. " +
				"Ask the user for the code, then call confirm_email.",
			Transports: ActsAsUser,
		}, handler.startEmailVerification),
		Define(Endpoint{
			Name:        "confirm_email",
			Description: "Confirm an email address with the code the user received (start_email_verification); then create_connection can use it.",
			Transports:  ActsAsUser,
		}, handler.confirmEmail),
		Define(Endpoint{
			Name: "create_connection",
			Description: "Add a notification connection for new and regressed issues: email (a confirmed address), telegram (bot_token, chat_id) " +
				"or slack (webhook_url, or slack_bot_token and slack_channel). Test it with send_test_message.",
			Transports: ActsAsUser, Confirm: true,
			Choices: connectionChoices,
		}, handler.createConnection),
		Define(Endpoint{
			Name:        "update_connection",
			Description: "Change a connection; fields not given keep their values (tokens too). Saving an active connection retries its waiting messages.",
			Idempotent:  true, Transports: ActsAsUser, Confirm: true,
			Choices: connectionChoices,
		}, handler.updateConnection),
		Define(Endpoint{
			Name:        "set_connection_active",
			Description: "Turn a connection on or off without changing anything else; an unconfirmed address cannot be turned on.",
			Idempotent:  true, Transports: ActsAsUser, Confirm: true,
		}, handler.setConnectionActive),
		Define(Endpoint{
			Name:        "test_chat",
			Description: "Send a test message with the chat fields of the form before they are saved; with connection_id, empty fields keep its saved tokens.",
			Transports:  TransportUI,
		}, handler.testChat),
		Define(Endpoint{
			Name:        "send_test_message",
			Description: "Send a test message through a saved connection and say whether it was delivered.",
			Transports:  ActsAsUser,
		}, handler.sendTestMessage),
		Define(Endpoint{
			Name:        "retry_connection",
			Description: "Send the messages of a connection that wait for a retry now, after the cause was fixed outside this server.",
			Idempotent:  true, Transports: ActsAsUser,
		}, handler.retryConnection),
		Define(Endpoint{
			Name:        "delete_connection",
			Description: "Delete a notification connection; its waiting messages are not sent. A confirmed address stays confirmed.",
			Destructive: true, Transports: ActsAsUser,
		}, handler.deleteConnection),
		Define(Endpoint{
			Name:        "list_deliveries",
			Description: "The delivery log, newest first: each message with its connection, issue, status, attempts and last error.",
			ReadOnly:    true, Transports: ActsAsUser,
			Choices: map[string][]string{"status": store.OutboxStatuses},
		}, handler.listDeliveries),
		Define(Endpoint{
			Name:        "count_deliveries",
			Description: "How many messages of the delivery log match the filters of list_deliveries.",
			ReadOnly:    true, Transports: TransportUI,
			Choices: map[string][]string{"status": store.OutboxStatuses},
		}, handler.countDeliveries),
	}
}

type deliveryHealth struct {
	Pending    int        `json:"pending,omitempty"`
	Retrying   int        `json:"retrying,omitempty"`
	Failed     int        `json:"failed,omitempty"`
	LastSentAt *time.Time `json:"last_sent_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	// Failing: messages wait for a retry, or the latest outcome was an error.
	Failing bool `json:"failing,omitempty"`
}

type connectionOutput struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Email       string   `json:"email,omitempty"`
	Destination string   `json:"destination"`
	AllProjects bool     `json:"all_projects"`
	Projects    []string `json:"projects,omitempty"`
	// ProjectIDs are the ids of Projects, in their order (the web app links each to its settings).
	ProjectIDs       []int           `json:"project_ids,omitempty"`
	Levels           []string        `json:"levels"`
	Active           bool            `json:"active"`
	Confirmed        bool            `json:"confirmed"`
	IncludeIssueLink bool            `json:"include_issue_link"`
	Delivery         *deliveryHealth `json:"delivery,omitempty"`
}

func (handler *handlers) connectionView(call Call, connection store.Connection, health map[string]store.DeliveryHealth) connectionOutput {
	output := connectionOutput{
		ID: connection.ID, Name: connection.Name, Type: connection.Type, AllProjects: connection.AllProjects, Levels: connection.Levels,
		Destination: handler.services.Connections.Destination(connection),
		Active:      connection.Active, Confirmed: connection.Verified, IncludeIssueLink: !connection.HideIssueLink,
	}

	if connection.Type == config.ChannelEmail {
		output.Email = connection.Address
	}

	for _, id := range connection.ProjectIDs {
		output.Projects = append(output.Projects, handler.projectName(id))
		output.ProjectIDs = append(output.ProjectIDs, id)
	}

	if state, ok := health[connection.ID]; ok {
		output.Delivery = &deliveryHealth{
			Pending: state.Pending, Retrying: state.Retrying, Failed: state.Failed, LastError: call.Text(state.LastError), Failing: state.Failing(),
		}

		if !state.LastSentAt.IsZero() {
			output.Delivery.LastSentAt = &state.LastSentAt
		}
	}

	return output
}

func (handler *handlers) listConnections(ctx context.Context, call Call, _ noInput) (Page[connectionOutput], error) {
	output := Page[connectionOutput]{Items: []connectionOutput{}}
	connections, err := handler.services.Connections.List(ctx, call.Access())

	if err != nil {
		return output, err
	}

	health, err := handler.services.Connections.DeliveryHealth(ctx, connections...)

	if err != nil {
		return output, err
	}

	for _, connection := range connections {
		output.Items = append(output.Items, handler.connectionView(call, connection, health))
	}

	return output, nil
}

// connectionDetailsOutput is a connection as its form shows it. Saved tokens
// are masked (textutil.MaskSecret), never whole.
type connectionDetailsOutput struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Type             string     `json:"type"`
	Email            string     `json:"email,omitempty"`
	Confirmed        bool       `json:"confirmed"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	AllProjects      bool       `json:"all_projects"`
	ProjectIDs       []int      `json:"project_ids"`
	Levels           []string   `json:"levels"`
	Active           bool       `json:"active"`
	IncludeIssueLink bool       `json:"include_issue_link"`
	ChatID           string     `json:"chat_id,omitempty"`
	SlackChannel     string     `json:"slack_channel,omitempty"`
	// The saved tokens, masked: what the form says is kept when its field stays empty.
	SavedTelegramToken string `json:"saved_telegram_token,omitempty"`
	SavedWebhookURL    string `json:"saved_webhook_url,omitempty"`
	SavedSlackBotToken string `json:"saved_slack_bot_token,omitempty"`
	// SecretReadable is false when the saved tokens cannot be decrypted (SECRET_KEY changed).
	SecretReadable bool `json:"secret_readable"`
	// Retrying is how many messages wait for a retry after a failed attempt.
	Retrying int `json:"retrying"`
}

func (handler *handlers) getConnection(ctx context.Context, call Call, input connectionInput) (connectionDetailsOutput, error) {
	connection, err := handler.services.Connections.Get(ctx, call.Access(), input.ConnectionID)

	if err != nil {
		return connectionDetailsOutput{}, err
	}

	output := connectionDetailsOutput{
		ID: connection.ID, Name: connection.Name, Type: connection.Type, Confirmed: connection.Verified,
		AllProjects: connection.AllProjects, ProjectIDs: append([]int{}, connection.ProjectIDs...), Levels: connection.Levels,
		Active: connection.Active, IncludeIssueLink: !connection.HideIssueLink, SecretReadable: true,
	}

	if connection.Type == config.ChannelEmail {
		output.Email = connection.Address

		if destination, err := handler.services.Verification.EmailDestination(ctx, connection.DestinationID); err == nil && destination.Verified {
			output.VerifiedAt = &destination.VerifiedAt
		}
	} else {
		chat := handler.services.Connections.Chat(connection)
		output.SecretReadable = chat.Readable

		if chat.Readable {
			output.ChatID, output.SavedTelegramToken = chat.Telegram.ChatID, textutil.MaskSecret(chat.Telegram.BotToken)
			output.SlackChannel = chat.Slack.Channel
			output.SavedWebhookURL, output.SavedSlackBotToken = textutil.MaskSecret(chat.Slack.WebhookURL), textutil.MaskSecret(chat.Slack.BotToken)
		}
	}

	health, err := handler.services.Connections.DeliveryHealth(ctx, connection)

	if err != nil {
		return output, err
	}

	output.Retrying = health[connection.ID].Retrying

	return output, nil
}

type channelTypeOutput struct {
	Type      string `json:"type"`
	Available bool   `json:"available"`
	// Reason is what is missing when it is not available: "set SMTP_HOST and EMAIL_FROM".
	Reason           string `json:"reason,omitempty"`
	IncludeIssueLink bool   `json:"include_issue_link"`
	NamePlaceholder  string `json:"name_placeholder"`
}

type connectionDefaultsOutput struct {
	Types []channelTypeOutput `json:"types"`
	// DefaultType is the first available type; empty when none is.
	DefaultType string   `json:"default_type"`
	Levels      []string `json:"levels"`
	// AllProjects and ProjectIDs are the projects a new connection starts
	// with: every one, or the viewer's when they have some.
	AllProjects bool  `json:"all_projects"`
	ProjectIDs  []int `json:"project_ids"`
	// IssueLinksAvailable: PUBLIC_URL is set, so messages can link to the issue.
	IssueLinksAvailable bool `json:"issue_links_available"`
}

func (handler *handlers) getConnectionDefaults(_ context.Context, call Call, _ noInput) (connectionDefaultsOutput, error) {
	connections := handler.services.Connections
	output := connectionDefaultsOutput{IssueLinksAvailable: connections.IssueLinksAvailable()}

	for _, channelType := range connections.Types() {
		output.Types = append(output.Types, channelTypeOutput{
			Type: channelType.Type, Available: channelType.Available, Reason: channelType.Reason,
			IncludeIssueLink: channelType.IncludeIssueLink, NamePlaceholder: channelType.NamePlaceholder,
		})

		if channelType.Available && output.DefaultType == "" {
			output.DefaultType = channelType.Type
		}
	}

	defaults := connections.New(output.DefaultType, call.Access())
	output.Levels, output.AllProjects, output.ProjectIDs = defaults.Levels, defaults.AllProjects, append([]int{}, defaults.ProjectIDs...)

	return output, nil
}

type emailInput struct {
	Email string `json:"email" jsonschema:"The email address."`
}

type emailStepOutput struct {
	Email     string `json:"email"`
	Confirmed bool   `json:"confirmed"`
	// State is code (a code was mailed, or one was sent moments ago) or verified.
	State      string     `json:"state"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	// Notice is what the page says of the step: "We sent a 6-digit code to …".
	Notice  string `json:"notice,omitempty"`
	Message string `json:"message"`
}

// emailStep tells what the step came to; a refused one names the input
// field it is about (field).
func emailStep(step services.EmailStep, field string) (emailStepOutput, error) {
	output := emailStepOutput{Email: step.Address, Confirmed: step.Verified(), State: step.State, Notice: step.Notice}

	if !step.VerifiedAt.IsZero() {
		output.VerifiedAt = &step.VerifiedAt
	}

	switch {
	case step.Duplicate != nil && step.Duplicate.ID == "":
		return output, InputError{status: http.StatusConflict, message: hiddenDuplicate, fields: map[string]string{field: hiddenDuplicate}}
	case step.Duplicate != nil:
		return output, duplicateError(*step.Duplicate, "sends to this address")
	case step.Unsent:
		// The mail server's failure, not the input's.
		return output, InputError{status: http.StatusBadGateway, message: step.Error, fields: map[string]string{field: step.Error}}
	case step.Error != "":
		return output, InputError{message: step.Error, fields: map[string]string{field: step.Error}}
	case output.Confirmed:
		output.Message = "Confirmed: create_connection can use this address."
	default:
		output.Message = "A code was mailed to the address: ask the user for it and call confirm_email."
	}

	if step.Notice != "" && !output.Confirmed {
		output.Message = step.Notice
	}

	return output, nil
}

func (handler *handlers) startEmailVerification(ctx context.Context, call Call, input emailInput) (emailStepOutput, error) {
	step, err := handler.services.Verification.StartEmail(ctx, call.Access(), input.Email)

	if err != nil {
		return emailStepOutput{}, err
	}

	return emailStep(step, "email")
}

type confirmEmailInput struct {
	Email string `json:"email" jsonschema:"The address the code was mailed to."`
	Code  string `json:"code" jsonschema:"The 6-digit code from the email."`
}

func (handler *handlers) confirmEmail(ctx context.Context, call Call, input confirmEmailInput) (emailStepOutput, error) {
	destination, ok, err := handler.services.Verification.FindDestination(ctx, input.Email)

	if err != nil {
		return emailStepOutput{}, err
	}

	if !ok {
		return emailStepOutput{}, notFound("no code was sent to %s: call start_email_verification first", input.Email)
	}

	step, err := handler.services.Verification.ConfirmCode(ctx, destination.ID, input.Code)

	if err != nil {
		return emailStepOutput{}, err
	}

	output, err := emailStep(step, "code")

	if err == nil && output.Confirmed && !destination.Verified {
		handler.audit(ctx, call, auditChange{
			Resource: store.AuditConnection, Action: store.AuditConfirm, RecordID: destination.ID, RecordName: destination.Address,
			Before: map[string]bool{"confirmed": false}, After: map[string]bool{"confirmed": true},
		})
	}

	return output, err
}

// connectionFields are the rule fields of a connection; nil keeps the value.
type connectionFields struct {
	Name        *string   `json:"name,omitempty" jsonschema:"A name; empty names it after the address or chat."`
	AllProjects *bool     `json:"all_projects,omitempty" jsonschema:"Every project, including future ones; false sends only for projects."`
	Projects    *[]string `json:"projects,omitempty" jsonschema:"Project names or IDs; an empty list means every project, unless all_projects is false."`
	Levels      *[]string `json:"levels,omitempty" jsonschema:"Issue levels to notify about (default: the levels recorded by default)."`
	Active      *bool     `json:"active,omitempty" jsonschema:"Send notifications (default true)."`
	// The default is store.NewConnection's.
	IncludeIssueLink *bool `json:"include_issue_link,omitempty" jsonschema:"Link to the issue in this panel (default true for email, false for chats, which may be public)."`
}

var connectionChoices = map[string][]string{"levels": config.Levels}

// chatFields are the tokens of a chat; empty keeps the stored ones.
type chatFields struct {
	BotToken      string `json:"bot_token,omitempty" jsonschema:"telegram: the bot token from @BotFather."`
	ChatID        string `json:"chat_id,omitempty" jsonschema:"telegram: the chat ID (negative for groups) or @channel."`
	WebhookURL    string `json:"webhook_url,omitempty" jsonschema:"slack: an incoming webhook URL."`
	SlackBotToken string `json:"slack_bot_token,omitempty" jsonschema:"slack: a bot token (xoxb-...) instead of a webhook; the bot also posts the agent prompt in a thread."`
	SlackChannel  string `json:"slack_channel,omitempty" jsonschema:"slack: the channel ID the bot is invited to."`
}

// replaced names the tokens given, which replace the stored ones.
func (chat chatFields) replaced() []string {
	if chat.BotToken != "" || chat.WebhookURL != "" || chat.SlackBotToken != "" {
		return []string{"token"}
	}

	return nil
}

// set writes the filled chat fields over the form.
func (chat chatFields) set(form url.Values) {
	for name, value := range map[string]string{
		"bot_token": chat.BotToken, "chat_id": chat.ChatID, "webhook_url": chat.WebhookURL,
		"slack_bot_token": chat.SlackBotToken, "slack_channel": chat.SlackChannel,
	} {
		if value != "" {
			form.Set(name, value)
		}
	}
}

// setCheckbox: a checkbox of the form as given; nil keeps it.
func setCheckbox(form url.Values, name string, value *bool) {
	switch {
	case value == nil:
	case *value:
		form.Set(name, "on")
	default:
		form.Del(name)
	}
}

// applyConnection writes the given fields over the form (projects:
// projectsField's refusals).
func (handler *handlers) applyConnection(call Call, form url.Values, fields connectionFields, chat chatFields) error {
	if fields.Name != nil {
		form.Set("name", *fields.Name)
	}

	if fields.Projects != nil {
		ids, err := handler.projectsField(call, "projects", *fields.Projects)

		if err != nil {
			return err
		}

		form.Del("project")

		for _, id := range ids {
			form.Add("project", strconv.Itoa(id))
		}
	}

	switch {
	case fields.AllProjects != nil:
		setCheckbox(form, "all_projects", fields.AllProjects)
	case fields.Projects != nil:
		all := len(*fields.Projects) == 0
		setCheckbox(form, "all_projects", &all)
	}

	if fields.Levels != nil {
		form["level"] = *fields.Levels
	}

	setCheckbox(form, "active", fields.Active)
	setCheckbox(form, "include_issue_link", fields.IncludeIssueLink)
	chat.set(form)

	return nil
}

// connectionFormFields names the form's fields as the tools do.
var connectionFormFields = map[string]string{"address": "email"}

// saveConnection saves the form over connection (none: a new one); replaced
// names the secrets the form replaces, for the audit log.
func (handler *handlers) saveConnection(ctx context.Context, call Call, form url.Values, connection store.Connection, replaced []string) (connectionOutput, error) {
	saved, err := handler.services.Connections.Save(ctx, call.Access(), form, connection)

	if err != nil {
		return connectionOutput{}, formError(err, connectionFormFields)
	}

	if saved.Duplicate != nil {
		return connectionOutput{}, duplicateError(*saved.Duplicate, "sends there")
	}

	if err := problemsError(saved.Problems, connectionFormFields); err != nil {
		return connectionOutput{}, err
	}

	// As stored, with its new secret.
	stored, err := handler.services.Connections.Get(ctx, store.Access{}, saved.Connection.ID)

	if err != nil {
		stored = saved.Connection
	}

	if connection.ID == "" {
		handler.audit(ctx, call, connectionChange(store.AuditCreate, stored, nil, handler.connectionAudit(stored)))
	} else {
		change := connectionChange(store.AuditEdit, stored, handler.connectionAudit(connection), handler.connectionAudit(stored))
		change.Replaced = replaced
		handler.audit(ctx, call, change)
	}

	return handler.connectionView(call, saved.Connection, nil), nil
}

type createConnectionInput struct {
	Type  string `json:"type" jsonschema:"email, telegram or slack."`
	Email string `json:"email,omitempty" jsonschema:"email: an address confirmed with start_email_verification and confirm_email."`
	connectionFields
	chatFields
}

func (handler *handlers) createConnection(ctx context.Context, call Call, input createConnectionInput) (connectionOutput, error) {
	// A new connection starts as in the UI (store.NewConnection).
	form := handler.services.Connections.FormValues(handler.services.Connections.New(input.Type, call.Access()))
	form.Set("type", input.Type)

	if input.Type == config.ChannelEmail {
		destination, ok, err := handler.services.Verification.FindDestination(ctx, input.Email)

		if err != nil {
			return connectionOutput{}, err
		}

		if !ok || !destination.Verified {
			return connectionOutput{}, unconfirmed("email", "confirm "+strconv.Quote(input.Email)+" first with start_email_verification and confirm_email")
		}

		form.Set("destination_id", destination.ID)
	}

	if err := handler.applyConnection(call, form, input.connectionFields, input.chatFields); err != nil {
		return connectionOutput{}, err
	}

	return handler.saveConnection(ctx, call, form, store.Connection{}, nil)
}

type connectionInput struct {
	ConnectionID string `json:"connection_id" jsonschema:"id from list_connections."`
}

type updateConnectionInput struct {
	ConnectionID string `json:"connection_id" jsonschema:"id from list_connections."`
	connectionFields
	chatFields
}

func (handler *handlers) updateConnection(ctx context.Context, call Call, input updateConnectionInput) (connectionOutput, error) {
	connection, err := handler.services.Connections.Get(ctx, call.Access(), input.ConnectionID)

	if err != nil {
		return connectionOutput{}, err
	}

	form := handler.services.Connections.FormValues(connection)

	if err := handler.applyConnection(call, form, input.connectionFields, input.chatFields); err != nil {
		return connectionOutput{}, err
	}

	return handler.saveConnection(ctx, call, form, connection, input.replaced())
}

type setConnectionActiveInput struct {
	ConnectionID string `json:"connection_id" jsonschema:"id from list_connections."`
	Active       bool   `json:"active" jsonschema:"true sends notifications, false stops them."`
}

// setConnectionActive changes only active: a save of the whole form would
// refuse a connection whose tokens can no longer be read, so it could not
// be turned off.
func (handler *handlers) setConnectionActive(ctx context.Context, call Call, input setConnectionActiveInput) (connectionOutput, error) {
	connection, err := handler.services.Connections.Get(ctx, call.Access(), input.ConnectionID)

	if err != nil {
		return connectionOutput{}, err
	}

	if err := handler.services.Connections.SetActive(ctx, connection, input.Active); errors.Is(err, store.ErrNotVerified) {
		return connectionOutput{}, unconfirmed("active", "Confirm the email address first.")
	} else if err != nil {
		return connectionOutput{}, err
	}

	before := connection.Active
	connection.Active = input.Active
	action := store.AuditResume

	if !input.Active {
		action = store.AuditPause
	}

	handler.audit(ctx, call, connectionChange(action, connection, map[string]bool{"active": before}, map[string]bool{"active": input.Active}))

	return handler.connectionView(call, connection, nil), nil
}

type testChatInput struct {
	Type             string `json:"type,omitempty" jsonschema:"telegram or slack; with connection_id, its own type."`
	ConnectionID     string `json:"connection_id,omitempty" jsonschema:"A saved connection: its tokens fill the empty fields."`
	Name             string `json:"name,omitempty" jsonschema:"The name the test message shows."`
	IncludeIssueLink bool   `json:"include_issue_link,omitempty" jsonschema:"As the form has it."`
	chatFields
}

type chatTestOutput struct {
	Sent bool `json:"sent"`
	// Message says what happened: sent, not delivered and why, or what is wrong in the fields.
	Message string `json:"message"`
}

func (handler *handlers) testChat(ctx context.Context, call Call, input testChatInput) (chatTestOutput, error) {
	var existing store.Connection

	channelType := input.Type

	if input.ConnectionID != "" {
		connection, err := handler.services.Connections.Get(ctx, call.Access(), input.ConnectionID)

		if err != nil {
			return chatTestOutput{}, err
		}

		existing, channelType = connection, connection.Type
	}

	form := url.Values{"name": {input.Name}}
	setCheckbox(form, "include_issue_link", &input.IncludeIssueLink)
	input.set(form)

	result := handler.services.Connections.TestChat(ctx, form, channelType, existing)

	return chatTestOutput{Sent: result.Sent, Message: result.Message}, nil
}

func (handler *handlers) sendTestMessage(ctx context.Context, call Call, input connectionInput) (doneOutput, error) {
	connection, err := handler.services.Connections.Get(ctx, call.Access(), input.ConnectionID)

	if err != nil {
		return doneOutput{}, err
	}

	var delivery *notify.DeliveryError

	if err := handler.services.Connections.SendTest(ctx, connection); errors.Is(err, store.ErrNotVerified) {
		return doneOutput{}, conflict("the address is not confirmed: no test message is sent")
	} else if errors.As(err, &delivery) {
		// The destination refused it: told as it is, not a failure of this server.
		return doneOutput{}, InputError{status: http.StatusBadGateway, message: "not delivered: " + call.Text(err.Error())}
	} else if err != nil {
		return doneOutput{}, err
	}

	return doneOutput{Done: "test message sent: check the destination"}, nil
}

// unconfirmed refuses at field what an unconfirmed address may not do (409).
func unconfirmed(field, problem string) error {
	input, _ := problemsError(map[string]string{field: problem}, nil).(InputError)
	input.status = http.StatusConflict

	return input
}

func (handler *handlers) retryConnection(ctx context.Context, call Call, input connectionInput) (doneOutput, error) {
	connection, err := handler.services.Connections.Get(ctx, call.Access(), input.ConnectionID)

	if err != nil {
		return doneOutput{}, err
	}

	if err := handler.services.Connections.RetryNow(ctx, call.Access(), input.ConnectionID); err != nil {
		return doneOutput{}, err
	}

	retry := connectionChange(store.AuditRetry, connection, nil, nil)
	retry.Always = true
	handler.audit(ctx, call, retry)

	return doneOutput{Done: "the waiting messages are being sent now"}, nil
}

func (handler *handlers) deleteConnection(ctx context.Context, call Call, input connectionInput) (doneOutput, error) {
	connection, err := handler.services.Connections.Get(ctx, call.Access(), input.ConnectionID)

	if err != nil {
		return doneOutput{}, err
	}

	if err := handler.services.Connections.Delete(ctx, call.Access(), input.ConnectionID); err != nil {
		return doneOutput{}, err
	}

	handler.audit(ctx, call, connectionChange(store.AuditDelete, connection, handler.connectionAudit(connection), nil))

	return doneOutput{Done: "deleted"}, nil
}

type deliveriesFilter struct {
	ConnectionID string `json:"connection_id,omitempty" jsonschema:"Only this connection."`
	Status       string `json:"status,omitempty" jsonschema:"Delivery status; every status when empty."`
}

func (filter deliveriesFilter) outbox() store.OutboxFilter {
	return store.OutboxFilter{Channel: filter.ConnectionID, Status: filter.Status}
}

type listDeliveriesInput struct {
	deliveriesFilter
	Limit  int    `json:"limit,omitempty" jsonschema:"Messages per page."`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page."`
}

// deliveryItemOutput is an issue a message is about; a summary lists several.
type deliveryItemOutput struct {
	Message string `json:"message"`
	Project string `json:"project,omitempty"`
	Title   string `json:"title,omitempty"`
	IssueID string `json:"issue_id,omitempty"`
}

type deliveryOutput struct {
	ID           int64  `json:"id"`
	ConnectionID string `json:"connection_id"`
	deliveryItemOutput
	// Items are the first issues of a summary, More the ones not listed.
	Items     []deliveryItemOutput `json:"items,omitempty"`
	More      int                  `json:"more,omitempty"`
	Status    string               `json:"status"`
	Retrying  bool                 `json:"retrying"`
	Attempts  int                  `json:"attempts"`
	LastError string               `json:"last_error,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
	// UpdatedAt is when it was sent, for a sent message.
	UpdatedAt     time.Time `json:"updated_at"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
}

func deliveryItem(call Call, description notify.Description) deliveryItemOutput {
	return deliveryItemOutput{Message: description.Label, Project: description.Project, Title: call.Text(description.Title), IssueID: description.IssueID}
}

func (handler *handlers) listDeliveries(ctx context.Context, call Call, input listDeliveriesInput) (Page[deliveryOutput], error) {
	output := Page[deliveryOutput]{Items: []deliveryOutput{}}

	limit := pageLimit(input.Limit)
	offset, err := parseOffset(input.Cursor)

	if err != nil {
		return output, err
	}

	filter := input.outbox()
	filter.Limit, filter.Offset = limit+1, offset

	records, err := handler.services.Deliveries.List(ctx, call.Access(), filter)

	if err != nil {
		return output, err
	}

	records, more := store.Page(records, limit)

	for _, record := range records {
		message := notify.Describe(record.Kind, record.Payload)
		row := deliveryOutput{
			ID: record.ID, ConnectionID: record.Channel, deliveryItemOutput: deliveryItem(call, message), More: message.More,
			Status: record.Status, Retrying: record.Retrying(), Attempts: record.Attempts, LastError: call.Text(record.LastError),
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, NextAttemptAt: record.NextAttemptAt,
		}

		for _, item := range message.Items {
			row.Items = append(row.Items, deliveryItem(call, item))
		}

		output.Items = append(output.Items, row)
	}

	if more {
		output.NextCursor = strconv.Itoa(offset + limit)
	}

	return output, nil
}

func (handler *handlers) countDeliveries(ctx context.Context, call Call, input deliveriesFilter) (countOutput, error) {
	count, err := handler.services.Deliveries.Count(ctx, call.Access(), input.outbox())

	return countOutput{Count: count}, err
}
