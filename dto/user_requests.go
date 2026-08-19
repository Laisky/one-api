package dto

// This file holds the inbound request DTOs for the three handlers that used to
// bind JSON directly into model.User and read its secret fields
// (Password/VerificationCode). Binding into these DTOs instead lets model.User's
// secret fields become json:"-" (deny-by-default outbound) without breaking
// inbound parsing — see docs/proposals/20260714_boundary-response-dtos.md §3.2.
//
// The four validate-tagged fields on model.User are Username (max=30),
// Password (min=8,max=20), DisplayName (max=20) and Email (max=50); no other
// User field carries a validate tag. Each DTO below replicates exactly those
// tags on the fields it exposes, so common.Validate.Struct behaves byte-for-byte
// identically to validating the whole model.User (T7).

// UserRegisterRequest is the inbound payload for password self-registration
// (controller.Register).
type UserRegisterRequest struct {
	Username         string `json:"username" validate:"max=30"`
	Password         string `json:"password" validate:"min=8,max=20"`
	DisplayName      string `json:"display_name" validate:"max=20"`
	Email            string `json:"email" validate:"max=50"`
	VerificationCode string `json:"verification_code"`
	// AffCode is the inviter's affiliate code (not the registrant's own code).
	AffCode string `json:"aff_code"`
}

// UserCreateRequest is the inbound payload for admin user creation
// (controller.CreateUser).
type UserCreateRequest struct {
	Username    string `json:"username" validate:"max=30"`
	Password    string `json:"password" validate:"min=8,max=20"`
	DisplayName string `json:"display_name" validate:"max=20"`
	Email       string `json:"email" validate:"max=50"`
	Role        int    `json:"role"`
	Quota       int64  `json:"quota"`
	Group       string `json:"group"`
}

// UserSelfUpdateRequest is the inbound payload for a user updating their own
// profile (controller.UpdateSelf). Plain string fields are used because the
// handler distinguishes "display_name omitted" from "display_name empty" by
// re-reading the raw request body (not by pointer presence), so that behavior
// is preserved unchanged.
type UserSelfUpdateRequest struct {
	Username    string `json:"username" validate:"max=30"`
	Password    string `json:"password" validate:"min=8,max=20"`
	DisplayName string `json:"display_name" validate:"max=20"`
	Email       string `json:"email" validate:"max=50"`
}
