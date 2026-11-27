package docs

import (
	"strings"
	"testing"
)

func TestFieldsAndSettings(t *testing.T) {
	fields := Fields()

	if text := fields["Notifications"]["Chat ID"]; !strings.HasPrefix(text, "The chat the bot writes to") {
		t.Errorf("Chat ID: %q", text)
	}

	// Fields are in their own section; headers are not fields.
	if _, ok := fields["Projects"]["Chat ID"]; ok {
		t.Error("Chat ID is a field of Projects")
	}

	if _, ok := fields["Notifications"]["Field"]; ok {
		t.Error("a header row is a field")
	}

	settings, err := Settings()

	if err != nil || settings["NOTIFY_KEEP_SENT"] != "How long sent messages stay in the delivery log." {
		t.Errorf("NOTIFY_KEEP_SENT: %q %v", settings["NOTIFY_KEEP_SENT"], err)
	}

	// Markdown is dropped: the text goes into a tooltip.
	if text := settings["ADMIN_PASSWORD"]; text == "" || strings.ContainsAny(text, "`[]") {
		t.Errorf("ADMIN_PASSWORD: %q", text)
	}
}
