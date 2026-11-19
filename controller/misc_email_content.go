package controller

import (
	"fmt"
	"net/url"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/message"
)

// emailVerificationContent renders the verification email body.
//
// Parameters:
//   - subject: the plain-text subject, reused as the heading.
//   - code: the generated verification code.
//
// Returns: the HTML body; the operator SystemName and the code are HTML-escaped.
func emailVerificationContent(subject, code string) string {
	return message.EmailTemplate(subject, fmt.Sprintf(`
			<p>Hello!</p>
			<p>You are verifying your email for %s.</p>
			<p>Your verification code is:</p>
			<p style="font-size: 24px; font-weight: bold; color: #333; background-color: #f8f8f8; padding: 10px; text-align: center; border-radius: 4px;">%s</p>
			<p style="color: #666;">The verification code is valid for %d minutes. If you did not request this, please ignore.</p>
		`, message.EscapeHTML(config.SystemName), message.EscapeHTML(code), common.VerificationValidMinutes))
}

// passwordResetLink builds the password-reset URL for an address and token.
//
// Parameters:
//   - email: the registered address the link resets; it is query-encoded, so
//     "+", "&" or quotes in a valid address survive the round trip.
//   - code: the reset token.
//
// Returns: "<ServerAddress>/user/reset?email=...&token=...".
func passwordResetLink(email, code string) string {
	query := url.Values{"email": {email}, "token": {code}}
	return config.ServerAddress + "/user/reset?" + query.Encode()
}

// passwordResetEmailContent renders the password-reset email body.
//
// Parameters:
//   - subject: the plain-text subject, reused as the heading.
//   - email: the registered address the link resets.
//   - code: the reset token.
//
// Returns: the HTML body. The link is HTML-escaped wherever it appears, so a
// valid but quote-bearing address (for example "a\"x"@example.com) cannot
// break out of the href attribute.
func passwordResetEmailContent(subject, email, code string) string {
	link := message.EscapeHTML(passwordResetLink(email, code))
	return message.EmailTemplate(subject, fmt.Sprintf(`
			<p>Hello!</p>
			<p>You are resetting your password for %s.</p>
			<p>Please click the button below to reset your password:</p>
			<p style="text-align: center; margin: 30px 0;">
				<a href="%s" style="background-color: #007bff; color: white; padding: 12px 24px; text-decoration: none; border-radius: 4px; display: inline-block;">Reset Password</a>
			</p>
			<p style="color: #666;">If the button doesn't work, please copy the following link and paste it into your browser:</p>
			<p style="background-color: #f8f8f8; padding: 10px; border-radius: 4px; word-break: break-all;">%s</p>
			<p style="color: #666;">The reset link is valid for %d minutes. If you didn't request this, please ignore.</p>
		`, message.EscapeHTML(config.SystemName), link, link, common.VerificationValidMinutes))
}
