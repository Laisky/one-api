package model

import (
	"strings"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/dto"
)

// ToResponse builds the external boundary DTO for an API token. It replaces the
// retired Token.MarshalJSON whitelist and carries the key-prefix normalization
// verbatim: the stored key is stripped of any known legacy prefix and the
// configured prefix is applied at response time only (the DB value is not
// modified). No internal integer id or user_id crosses the API.
//
// Parameters: none (pointer receiver).
//
// Return values:
//   - dto.TokenResponse: the UUID-only external shape with a prefixed key. A
//     nil receiver yields the zero shape.
func (t *Token) ToResponse() dto.TokenResponse {
	if t == nil {
		return dto.TokenResponse{}
	}
	return dto.TokenResponse{
		UUID:           t.UUID,
		UserUUID:       t.UserUUID,
		Key:            normalizeTokenKeyForResponse(t.Key),
		Status:         t.Status,
		Name:           t.Name,
		CreatedTime:    t.CreatedTime,
		AccessedTime:   t.AccessedTime,
		ExpiredTime:    t.ExpiredTime,
		RemainQuota:    t.RemainQuota,
		UnlimitedQuota: t.UnlimitedQuota,
		UsedQuota:      t.UsedQuota,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		Models:         t.Models,
		Subnet:         t.Subnet,
	}
}

// normalizeTokenKeyForResponse strips any known legacy prefix from a stored key
// and applies the configured token key prefix (defaulting to "sk-"). This is the
// exact logic previously embedded in Token.MarshalJSON.
func normalizeTokenKeyForResponse(key string) string {
	raw := key
	raw = strings.TrimPrefix(raw, "sk-")
	raw = strings.TrimPrefix(raw, "laisky-")
	prefix := config.TokenKeyPrefix
	if prefix == "" {
		prefix = "sk-"
	}
	return prefix + raw
}

// TokensToResponses maps a slice of tokens to their external DTOs,
// pre-allocating the result.
//
// Parameters:
//   - tokens: rows to convert; nil elements map to the zero shape.
//
// Return values:
//   - []dto.TokenResponse: one entry per input row.
func TokensToResponses(tokens []*Token) []dto.TokenResponse {
	out := make([]dto.TokenResponse, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, t.ToResponse())
	}
	return out
}
