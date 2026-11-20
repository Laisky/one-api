package message

import (
	"net/mail"
	"strings"

	"github.com/Laisky/errors/v2"
)

// errInvalidEmailAddress reports a recipient or sender that is not a bare RFC
// 5322 address. It deliberately carries no part of the address, because email
// addresses are never logged and SendEmail errors are logged by callers.
var errInvalidEmailAddress = errors.New("not a bare RFC 5322 email address")

// emailAddress is a bare RFC 5322 address that net/mail parsed and re-serialized.
type emailAddress struct {
	// address is the parsed address; its Name is always empty.
	address *mail.Address
	// addrSpec is the canonical addr-spec (quoted where RFC 5322 requires) used
	// for SMTP envelope commands and the Resend recipient list.
	addrSpec string
}

// parseBareEmailAddress validates one bare address and returns its canonical form.
//
// Parameters:
//   - value: a single address such as "user@example.com"; surrounding whitespace
//     is ignored.
//
// Returns:
//   - emailAddress: the parsed address and its canonical addr-spec.
//   - error: errInvalidEmailAddress (wrapped) when the value contains a line
//     break, carries a display name or comment, or fails net/mail parsing.
func parseBareEmailAddress(value string) (emailAddress, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return emailAddress{}, errors.WithStack(errInvalidEmailAddress)
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		// The parser error can quote the address, so only its absence of
		// validity is reported; the error is fully handled here.
		return emailAddress{}, errors.Wrap(errInvalidEmailAddress, "net/mail rejected the address")
	}
	if parsed.Name != "" {
		return emailAddress{}, errors.Wrap(errInvalidEmailAddress, "display names are not accepted")
	}
	canonical := (&mail.Address{Address: parsed.Address}).String()
	return emailAddress{
		address:  &mail.Address{Address: parsed.Address},
		addrSpec: strings.TrimSuffix(strings.TrimPrefix(canonical, "<"), ">"),
	}, nil
}

// parseEmailRecipients parses a ";"-separated recipient list.
//
// Parameters:
//   - receiver: recipients separated by ";"; empty entries are skipped.
//
// Returns:
//   - []emailAddress: every recipient in order, each validated by
//     parseBareEmailAddress.
//   - error: when any entry is invalid (the whole send is refused, so a forged
//     entry never reaches a provider) or when no recipient remains.
func parseEmailRecipients(receiver string) ([]emailAddress, error) {
	var recipients []emailAddress
	position := 0
	for entry := range strings.SplitSeq(receiver, ";") {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		position++
		recipient, err := parseBareEmailAddress(entry)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid recipient #%d", position)
		}
		recipients = append(recipients, recipient)
	}
	if len(recipients) == 0 {
		return nil, errors.New("no valid recipient email addresses")
	}
	return recipients, nil
}

// formatToHeader renders the To header value from validated recipients.
// Parameters: recipients are parsed addresses. Returns: the RFC 5322 address
// list, such as "<a@example.com>, <b@example.com>".
func formatToHeader(recipients []emailAddress) string {
	rendered := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		rendered = append(rendered, recipient.address.String())
	}
	return strings.Join(rendered, ", ")
}

// formatFromHeader renders the From header value. net/mail quotes the display
// name, or RFC 2047 encodes it when it holds non-ASCII or control characters.
// Parameters: displayName is the operator's system name (may be empty) and
// sender is the validated sender. Returns: the RFC 5322 mailbox.
func formatFromHeader(displayName string, sender emailAddress) string {
	return (&mail.Address{Name: displayName, Address: sender.address.Address}).String()
}
