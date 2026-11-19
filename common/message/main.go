package message

import (
	"fmt"
	"html"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/config"
)

const (
	// ByAll dispatches notifications via both email and message pusher channels.
	ByAll = "all"
	// ByEmail delivers notifications exclusively through email.
	ByEmail = "email"
	// ByMessagePusher delivers notifications through the configured message pusher integration.
	ByMessagePusher = "message_pusher"
)

// Notify routes the notification to the requested channel(s) and returns any delivery errors.
//
// Parameters:
//   - by: ByAll, ByEmail or ByMessagePusher.
//   - title: plain-text title, used as the email subject.
//   - description: message pusher description.
//   - content: plain text, such as a channel-test error that may quote provider
//     output. It is HTML-escaped for the HTML email body and passed verbatim to
//     the message pusher.
//
// Returns: the delivery errors, joined for ByAll.
func Notify(by string, title string, description string, content string) error {
	emailBody := html.EscapeString(content)
	switch by {
	case ByAll:
		var errMsgs []string
		if err := SendEmail(title, config.RootUserEmail, emailBody); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("failed to send email: %v", err))
		}
		if err := SendMessage(title, description, content); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("failed to send message: %v", err))
		}

		if len(errMsgs) > 0 {
			return errors.Errorf("multiple errors occurred: %v", errMsgs)
		}
		return nil
	case ByEmail:
		return SendEmail(title, config.RootUserEmail, emailBody)
	case ByMessagePusher:
		return SendMessage(title, description, content)
	default:
		return errors.Errorf("unknown notify method: %s", by)
	}
}
