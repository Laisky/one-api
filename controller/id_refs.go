package controller

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/idresolve"
	"github.com/Laisky/one-api/model"
)

// resolveUserRef resolves a user UUID string to an internal id.
// Parameters:
//   - ref: client supplied user reference.
//
// Return values:
//   - int: internal user primary key.
//   - error: invalid-reference or not-found error.
func resolveUserRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetUserIdByUUID, ref)
}

// resolveOptionalUserRef resolves an optional user UUID string.
// Parameters:
//   - ref: client supplied user reference; empty or "0" means no filter.
//
// Return values:
//   - int: internal user primary key, or 0 when ref is empty.
//   - error: invalid-reference or not-found error.
func resolveOptionalUserRef(ref string) (int, error) {
	if isUnsetOptionalRef(ref) {
		return 0, nil
	}
	return resolveUserRef(ref)
}

// isUnsetOptionalRef reports whether an optional reference means "no filter".
// Parameters:
//   - ref: client supplied optional reference.
//
// Return values:
//   - bool: true for an empty value or the legacy integer sentinel "0", which
//     pre-UUID clients (and the bundled admin CLI) send to mean "all".
func isUnsetOptionalRef(ref string) bool {
	trimmed := strings.TrimSpace(ref)
	return trimmed == "" || trimmed == "0"
}

// resolveChannelRef resolves a channel UUID string to an internal id.
// Parameters:
//   - ref: client supplied channel reference.
//
// Return values:
//   - int: internal channel primary key.
//   - error: invalid-reference or not-found error.
func resolveChannelRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetChannelIdByUUID, ref)
}

// resolveOptionalChannelRef resolves an optional channel UUID string.
// Parameters:
//   - ref: client supplied channel reference; empty or "0" means no filter.
//
// Return values:
//   - int: internal channel primary key, or 0 when ref is empty.
//   - error: invalid-reference or not-found error.
func resolveOptionalChannelRef(ref string) (int, error) {
	if isUnsetOptionalRef(ref) {
		return 0, nil
	}
	return resolveChannelRef(ref)
}

// resolveTokenRef resolves a token UUID string to an internal id.
// Parameters:
//   - ref: client supplied token reference.
//
// Return values:
//   - int: internal token primary key.
//   - error: invalid-reference or not-found error.
func resolveTokenRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetTokenIdByUUID, ref)
}

// resolveRedemptionRef resolves a redemption UUID string to an internal id.
// Parameters:
//   - ref: client supplied redemption reference.
//
// Return values:
//   - int: internal redemption primary key.
//   - error: invalid-reference or not-found error.
func resolveRedemptionRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetRedemptionIdByUUID, ref)
}

// resolveMCPServerRef resolves an MCP server UUID string to an internal id.
// Parameters:
//   - ref: client supplied MCP server reference.
//
// Return values:
//   - int: internal MCP server primary key.
//   - error: invalid-reference or not-found error.
func resolveMCPServerRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetMCPServerIdByUUID, ref)
}

// resolveOptionalMCPServerRef resolves an optional MCP server UUID string.
// Parameters:
//   - ref: client supplied MCP server reference; empty or "0" means no filter.
//
// Return values:
//   - int: internal MCP server primary key, or 0 when ref is empty.
//   - error: invalid-reference or not-found error.
func resolveOptionalMCPServerRef(ref string) (int, error) {
	if isUnsetOptionalRef(ref) {
		return 0, nil
	}
	return resolveMCPServerRef(ref)
}

// resolveLogRef resolves a log UUID string to an internal id.
// Parameters:
//   - ref: client supplied log reference.
//
// Return values:
//   - int: internal log primary key.
//   - error: invalid-reference or not-found error.
func resolveLogRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetLogIdByUUID, ref)
}

// resolvePasskeyCredentialRef resolves a passkey credential UUID string to an internal id.
// Parameters:
//   - ref: client supplied passkey credential reference.
//
// Return values:
//   - int: internal passkey credential primary key.
//   - error: invalid-reference or not-found error.
func resolvePasskeyCredentialRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetPasskeyCredentialIdByUUID, ref)
}

// preferUUIDRef returns a UUID ref for strict-in request bodies.
// Parameters:
//   - uuid: client supplied UUID field.
//   - id: ignored legacy integer id field retained only for decoding old payloads.
//
// Return values:
//   - string: UUID reference string.
//   - error: errResourceUUIDRequired, classified as a client error, when UUID is empty.
func preferUUIDRef(uuid string, id int) (string, error) {
	if strings.TrimSpace(uuid) != "" {
		return uuid, nil
	}
	return "", errors.WithStack(errResourceUUIDRequired)
}

// errResourceUUIDRequired marks a strict-in request body that omits the
// resource uuid. It is client-caused by definition, so the mark makes the
// shared handler logger record it at WARN without a stack trace.
var errResourceUUIDRequired = errkind.InvalidRequestErr(errors.New("resource uuid is required"))

// isInvalidResourceRef reports whether err is a missing or malformed resource
// reference produced by this file. Not-found references and lookup failures
// (for example a database outage) are deliberately excluded.
// Parameters:
//   - err: error returned by a resolver or preferUUIDRef, possibly wrapped.
//
// Return values:
//   - bool: true for idresolve.ErrInvalidRef or errResourceUUIDRequired.
func isInvalidResourceRef(err error) bool {
	return errors.Is(err, idresolve.ErrInvalidRef) || errors.Is(err, errResourceUUIDRequired)
}

// respondResourceRefError writes the response for a failed reference
// resolution. A missing or malformed reference is the caller's mistake and
// answers HTTP 400 (logged at WARN without a stack). Every other failure keeps
// the /api/* convention of HTTP 200 with {"success":false}: a not-found
// reference stays a WARN-level client error and a lookup failure (for example
// a database outage) stays an ERROR.
// Parameters:
//   - c: request context.
//   - err: error returned by a resolver or preferUUIDRef; it selects the status
//     and log level and is written to the response body and the log.
func respondResourceRefError(c *gin.Context, err error) {
	if isInvalidResourceRef(err) {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, err)
		return
	}
	helper.RespondError(c, err)
}

// respondResourceRefErrorAs writes the response for a failed reference
// resolution on an endpoint that documents its own client message. Status and
// log level follow respondResourceRefError exactly; only the response body
// message is replaced, while the log keeps cause and its stack.
// Parameters:
//   - c: request context.
//   - cause: error returned by a resolver or preferUUIDRef.
//   - message: endpoint-specific client message written to the response body.
func respondResourceRefErrorAs(c *gin.Context, cause error, message string) {
	respondResourceRefError(c, &resourceRefMessageError{message: message, cause: cause})
}

// resourceRefMessageError presents an endpoint-specific client message for a
// failed reference resolution while keeping the resolver's error as its
// cause. Unwrap keeps errors.Is, errors.As and errkind attribution on the
// cause, so status and log level are unchanged by the message override.
type resourceRefMessageError struct {
	message string
	cause   error
}

// Error returns the client message only, so the cause never reaches the
// response body.
// Return values:
//   - string: the endpoint-specific client message.
func (e *resourceRefMessageError) Error() string { return e.message }

// Unwrap returns the resolver error for errors.Is, errors.As and errkind.Of.
// Return values:
//   - error: the wrapped resolver error.
func (e *resourceRefMessageError) Unwrap() error { return e.cause }

// Format prints the client message for %s, %v and %q, and appends the cause
// with its stack for %+v, which is what the ERROR log records as errorVerbose.
// Parameters:
//   - s: formatter state.
//   - verb: formatting verb.
func (e *resourceRefMessageError) Format(s fmt.State, verb rune) {
	switch {
	case verb == 'v' && s.Flag('+'):
		_, _ = fmt.Fprintf(s, "%s: %+v", e.message, e.cause)
	case verb == 'q':
		_, _ = fmt.Fprintf(s, "%q", e.message)
	default:
		_, _ = io.WriteString(s, e.message)
	}
}
