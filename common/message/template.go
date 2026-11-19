package message

import (
	"fmt"
	"html"

	"github.com/Laisky/one-api/common/config"
)

// EmailTemplate renders the shared HTML email layout.
//
// Parameters:
//   - title: plain text shown as the heading; it is HTML-escaped here.
//   - content: the body as trusted HTML markup. Callers own this markup and must
//     HTML-escape (html.EscapeString) every untrusted value they interpolate
//     into it, such as provider error text, user input, or channel names.
//
// Returns: the complete HTML document. The operator's SystemName in the footer
// is HTML-escaped as text.
func EmailTemplate(title, content string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin: 0; padding: 20px; font-family: Arial, sans-serif; line-height: 1.6; background-color: #f4f4f4;">
    <div style="max-width: 600px; margin: 20px auto; padding: 30px; background-color: #ffffff; border-radius: 8px; box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);">
        <div style="text-align: center; margin-bottom: 30px;">
            <h2 style="color: #333; margin: 0; font-size: 24px;">%s</h2>
        </div>
        <div style="color: #555; font-size: 16px;">
            %s
        </div>
        <div style="margin-top: 40px; padding-top: 20px; border-top: 1px solid #eee; color: #888; font-size: 14px; text-align: center;">
            <p style="margin: 5px 0;">This email was sent automatically by the system, please do not reply directly.</p>
            <p style="margin: 5px 0;">%s</p>
        </div>
    </div>
</body>
</html>
`, html.EscapeString(title), content, html.EscapeString(config.SystemName))
}
