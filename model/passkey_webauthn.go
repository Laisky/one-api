package model

import (
	"encoding/binary"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// WebAuthnUser adapts model.User to the webauthn.User interface.
type WebAuthnUser struct {
	User        *User
	Credentials []*PasskeyCredential
}

var _ webauthn.User = (*WebAuthnUser)(nil)

func (u *WebAuthnUser) WebAuthnID() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(u.User.Id))
	return buf
}

func (u *WebAuthnUser) WebAuthnName() string {
	return u.User.Username
}

func (u *WebAuthnUser) WebAuthnDisplayName() string {
	if u.User.DisplayName != "" {
		return u.User.DisplayName
	}
	return u.User.Username
}

func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	creds := make([]webauthn.Credential, 0, len(u.Credentials))
	for _, c := range u.Credentials {
		creds = append(creds, PasskeyCredentialToWebAuthn(c))
	}
	return creds
}

// PasskeyCredentialToWebAuthn converts a stored PasskeyCredential to the library's Credential type.
func PasskeyCredentialToWebAuthn(c *PasskeyCredential) webauthn.Credential {
	transports := parseTransports(c.Transport)
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.AAGUID,
			SignCount: c.SignCount,
		},
	}
}

func parseTransports(s string) []protocol.AuthenticatorTransport {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]protocol.AuthenticatorTransport, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, protocol.AuthenticatorTransport(p))
		}
	}
	return out
}

// TransportsToString serializes transport types to a comma-separated string for DB storage.
func TransportsToString(ts []protocol.AuthenticatorTransport) string {
	parts := make([]string, 0, len(ts))
	for _, t := range ts {
		parts = append(parts, string(t))
	}
	return strings.Join(parts, ",")
}

// NewWebAuthnUser builds a WebAuthnUser from a model.User, loading credentials from the DB.
func NewWebAuthnUser(user *User) (*WebAuthnUser, error) {
	creds, err := GetPasskeyCredentialsByUserId(user.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "get passkey credentials for user %d", user.Id)
	}
	return &WebAuthnUser{User: user, Credentials: creds}, nil
}
