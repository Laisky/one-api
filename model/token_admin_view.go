package model

import "github.com/Laisky/one-api/dto"

// ToAdminResponse projects token metadata into the secret-free administrator
// inventory. A nil receiver returns an empty DTO and never reads a token key.
func (t *Token) ToAdminResponse() dto.AdminTokenResponse {
	if t == nil {
		return dto.AdminTokenResponse{}
	}
	return dto.AdminTokenResponse{
		UUID:           t.UUID,
		UserUUID:       t.UserUUID,
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

// TokensToAdminResponses returns one secret-free inventory DTO per token.
func TokensToAdminResponses(tokens []*Token) []dto.AdminTokenResponse {
	out := make([]dto.AdminTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		out = append(out, token.ToAdminResponse())
	}
	return out
}
