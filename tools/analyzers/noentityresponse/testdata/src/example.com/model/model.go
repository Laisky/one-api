package model

import "example.com/dto"

// The five forbidden entity stubs (only the names and package matter).
type User struct{ Id int }
type Token struct{ Id int }
type Channel struct{ Id int }
type Redemption struct{ Id int }
type Log struct{ Id int }

// Safe is a non-forbidden type in the same package (negative case).
type Safe struct{ X int }

// ToResponse is the boundary mapper (negative case).
func (u *User) ToResponse() dto.UserResponse { return dto.UserResponse{} }

// UsersToResponses is the list mapper (negative case).
func UsersToResponses(us []*User) []dto.UserResponse { return nil }
