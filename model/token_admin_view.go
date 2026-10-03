package model

import "github.com/Laisky/one-api/dto"

// ToAdminResponse returns token metadata with the owner credential removed.
func (t *Token) ToAdminResponse() dto.TokenResponse {
	out := t.ToResponse()
	out.Key = ""
	return out
}

// TokensToAdminResponses maps rows to metadata-only administrator views.
func TokensToAdminResponses(tokens []*Token) []dto.TokenResponse {
	out := make([]dto.TokenResponse, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, t.ToAdminResponse())
	}
	return out
}
