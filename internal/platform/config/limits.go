package config

// FieldLimits are the limits of form fields for the pages (maxlength,
// minlength and pattern attributes; a pattern must match the whole value),
// so the forms and the server's checks never differ. The json names are the
// web app's (catalog.gen.ts).
type FieldLimits struct {
	ConnectionName     int    `json:"connectionName"`
	Username           int    `json:"username"`
	UsernamePattern    string `json:"usernamePattern"`
	DisplayName        int    `json:"displayName"`
	MinPassword        int    `json:"minPassword"`
	MaxPassword        int    `json:"maxPassword"`
	ProjectName        int    `json:"projectName"`
	ProjectNamePattern string `json:"projectNamePattern"`
	VerificationCode   int    `json:"verificationCode"`
	TelegramToken      string `json:"telegramToken"`
	TelegramChat       string `json:"telegramChat"`
	SlackToken         string `json:"slackToken"`
	SlackChannel       string `json:"slackChannel"`
}

var Limits = FieldLimits{
	ConnectionName: MaxConnectionName, Username: MaxUsername, UsernamePattern: UsernamePattern, DisplayName: MaxDisplayName,
	MinPassword: MinPasswordLength, MaxPassword: MaxPasswordLength, ProjectName: MaxProjectName, ProjectNamePattern: ProjectNamePattern,
	VerificationCode: VerificationCodeDigits, TelegramToken: TelegramTokenPattern, TelegramChat: TelegramChatPattern,
	SlackToken: SlackTokenPattern, SlackChannel: SlackChannelPattern,
}

// HTTP server limits: connections over MaxConnections wait in the accept
// queue; a request whose headers are larger than MaxHeaderBytes gets 431.
const (
	MaxConnections = 1000
	MaxHeaderBytes = 64 << 10
)
