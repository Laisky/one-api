package model

import "github.com/Laisky/one-api/dto"

// ToResponse builds the external boundary DTO for a redemption code. It is the
// explicit, boundary-invoked replacement for the retired Redemption.MarshalJSON
// whitelist: no internal integer id or user_id crosses the API.
//
// Parameters: none (pointer receiver).
//
// Return values:
//   - dto.RedemptionResponse: the UUID-only external shape. A nil receiver
//     yields the zero shape so wrapper code stays nil-safe.
func (redemption *Redemption) ToResponse() dto.RedemptionResponse {
	if redemption == nil {
		return dto.RedemptionResponse{}
	}
	return dto.RedemptionResponse{
		UUID:         redemption.UUID,
		UserUUID:     redemption.UserUUID,
		Key:          redemption.Key,
		Status:       redemption.Status,
		Name:         redemption.Name,
		Quota:        redemption.Quota,
		CreatedTime:  redemption.CreatedTime,
		RedeemedTime: redemption.RedeemedTime,
		Count:        redemption.Count,
		CreatedAt:    redemption.CreatedAt,
		UpdatedAt:    redemption.UpdatedAt,
	}
}

// RedemptionsToResponses maps a slice of redemptions to their external DTOs,
// pre-allocating the result.
//
// Parameters:
//   - redemptions: rows to convert; nil elements map to the zero shape.
//
// Return values:
//   - []dto.RedemptionResponse: one entry per input row (never nil for a
//     non-nil input).
func RedemptionsToResponses(redemptions []*Redemption) []dto.RedemptionResponse {
	out := make([]dto.RedemptionResponse, 0, len(redemptions))
	for _, r := range redemptions {
		out = append(out, r.ToResponse())
	}
	return out
}
