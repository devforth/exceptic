package services

import (
	"fmt"
	"strings"
)

// SDKSetup is how an app of one platform reports to a project: the one
// source of the installation code the project's setup page shows.
type SDKSetup struct {
	Platform string // its ID, such as "node"
	Label    string // "Node.js"
	Install  string // the command that installs the SDK
	// Where says where Init goes, as a sentence.
	Where string
	Init  string // the code that starts the SDK with the project's DSN
	Test  string // a line that sends a test error
	// AgentPrompt asks a coding agent to do the same in the user's app.
	AgentPrompt string
}

// dsnMark stands for the DSN in the snippets below.
const dsnMark = "{dsn}"

// sdkSetups are the platforms of the setup page, the official Sentry SDK of each.
var sdkSetups = []SDKSetup{
	{
		Platform: "node", Label: "Node.js", Install: "npm install @sentry/node",
		Where: "As early as possible, before the rest of the app loads.",
		Init: `// instrument.mjs: load it before the rest of the app,
// for example: node --import ./instrument.mjs app.mjs
import * as Sentry from "@sentry/node";

Sentry.init({
  dsn: "{dsn}",
  enableLogs: true,
});`,
		Test: `Sentry.captureException(new Error("Test error"));`,
	},
	{
		Platform: "browser", Label: "Browser", Install: "npm install @sentry/browser",
		Where: "In the entry file of the front end, before the app renders.",
		Init: `import * as Sentry from "@sentry/browser";

Sentry.init({
  dsn: "{dsn}",
  enableLogs: true,
});`,
		Test: `Sentry.captureException(new Error("Test error"));`,
	},
	{
		Platform: "python", Label: "Python", Install: "pip install sentry-sdk",
		Where: "At startup, before the web framework's app is created.",
		Init: `import sentry_sdk

sentry_sdk.init(
    dsn="{dsn}",
    enable_logs=True,
)`,
		Test: `sentry_sdk.capture_exception(Exception("Test error"))`,
	},
	{
		Platform: "go", Label: "Go", Install: "go get github.com/getsentry/sentry-go",
		Where: "First thing in main().",
		Init: `import (
	"time"

	"github.com/getsentry/sentry-go"
)

func main() {
	sentry.Init(sentry.ClientOptions{
		Dsn: "{dsn}",
	})
	defer sentry.Flush(2 * time.Second)
	// …
}`,
		Test: `sentry.CaptureMessage("Test error")`,
	},
	{
		Platform: "php", Label: "PHP", Install: "composer require sentry/sentry",
		Where: "In the bootstrap file, right after the autoloader.",
		Init: `\Sentry\init([
    'dsn' => '{dsn}',
]);`,
		Test: `\Sentry\captureMessage('Test error');`,
	},
	{
		Platform: "ruby", Label: "Ruby", Install: "bundle add sentry-ruby",
		Where: "In an initializer that runs at boot.",
		Init: `require "sentry-ruby"

Sentry.init do |config|
  config.dsn = "{dsn}"
end`,
		Test: `Sentry.capture_message("Test error")`,
	},
}

// SDKSetups are the installation steps of every platform with the DSN filled in.
func SDKSetups(dsn string) []SDKSetup {
	setups := make([]SDKSetup, 0, len(sdkSetups))

	for _, setup := range sdkSetups {
		setup.Init = strings.ReplaceAll(setup.Init, dsnMark, dsn)
		setup.AgentPrompt = fmt.Sprintf(`Connect this %s app to our error monitor. It is Sentry-compatible, so use the official Sentry SDK.
1. Install the SDK: %s
2. Initialize it at startup (%s) with this DSN:
   %s
   If the project keeps settings in environment variables, read it from SENTRY_DSN instead of hard-coding it.
3. Change nothing else. Run the app and send one test error to check the connection:
   %s
Then tell me which files you changed.`, setup.Label, setup.Install, lowerFirst(strings.TrimSuffix(setup.Where, ".")), dsn, setup.Test)
		setups = append(setups, setup)
	}

	return setups
}

func lowerFirst(text string) string {
	if text == "" {
		return text
	}

	return strings.ToLower(text[:1]) + text[1:]
}
