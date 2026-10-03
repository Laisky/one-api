package errkind

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// theReportedMessage is the exact text an operator saw logged at ERROR with a
// full stack trace for a user who had simply run out of quota.
const theReportedMessage = "insufficient user quota: required=50, available=0, userId=175, tokenId=257"

func TestMark_PreservesErrorTextByteIdentically(t *testing.T) {
	orig := errors.Errorf("insufficient user quota: required=%d, available=%d, userId=%d, tokenId=%d",
		50, 0, 175, 257)
	require.Equal(t, theReportedMessage, orig.Error())

	marked := Quota(orig)
	require.Equal(t, theReportedMessage, marked.Error())
	require.Equal(t, orig.Error(), marked.Error())
}

func TestMark_PreservesStackFormatting(t *testing.T) {
	orig := errors.Errorf("boom %d", 1)
	marked := ServerErr(orig)

	require.Equal(t, fmt.Sprintf("%+v", orig), fmt.Sprintf("%+v", marked))
	require.Equal(t, fmt.Sprintf("%v", orig), fmt.Sprintf("%v", marked))
	require.Equal(t, fmt.Sprintf("%q", orig), fmt.Sprintf("%q", marked))
	require.Contains(t, fmt.Sprintf("%+v", marked), "errkind.TestMark_PreservesStackFormatting")
}

func TestMark_TransparentToIsAs(t *testing.T) {
	sentinel := stderrors.New("record not found")
	marked := NotFoundErr(errors.Wrap(sentinel, "load token"))

	require.True(t, errors.Is(marked, sentinel))
	require.True(t, stderrors.Is(marked, sentinel))
	require.True(t, errors.Is(errors.Wrapf(marked, "outer"), sentinel))
}

// TestMark_TransparentToStringsContains guards the classifiers in
// controller/relay_error.go and middleware/utils.go, which decide retry
// behaviour and channel suspension from the message text.
func TestMark_TransparentToStringsContains(t *testing.T) {
	for _, probe := range []string{
		"insufficient user quota",
		"token not found for key:",
		"No available channels for Model",
	} {
		base := errors.Errorf("%s foo", probe)
		outer := errors.Wrapf(Quota(base), "wrapped")
		require.True(t, strings.Contains(outer.Error(), probe), probe)
	}
}

func TestMark_NilAndUnknownAreNoOps(t *testing.T) {
	require.Nil(t, Mark(nil, InsufficientQuota))
	require.Nil(t, Quota(nil))

	base := errors.New("boom")
	require.Equal(t, base, Mark(base, Unknown))
}

func TestOf_OutermostWins(t *testing.T) {
	inner := Quota(errors.New("inner"))
	middle := errors.Wrapf(inner, "wrapped")
	require.Equal(t, InsufficientQuota, Of(middle))

	// A caller with more context may reclassify.
	require.Equal(t, Server, Of(ServerErr(middle)))

	require.Equal(t, Unknown, Of(nil))
	require.Equal(t, Unknown, Of(errors.New("unmarked")))
}

func TestKind_IsClient(t *testing.T) {
	for _, k := range []Kind{InvalidRequest, Unauthorized, Forbidden, NotFound,
		InsufficientQuota, RateLimited, Conflict, Config} {
		require.Truef(t, k.IsClient(), "%s should be client-attributed", k)
	}
	for _, k := range []Kind{Upstream, Server} {
		require.Falsef(t, k.IsClient(), "%s must not be client-attributed", k)
	}
	// An unclassified error must keep the conservative treatment.
	require.False(t, Unknown.IsClient())
}

// TestLogAsWarn is the truth table for the reported bug and its mirror.
func TestLogAsWarn(t *testing.T) {
	quota := Quota(errors.New(theReportedMessage))
	serverFault := ServerErr(errors.New("dial tcp: connection refused"))
	unmarked := errors.New("something happened")

	for _, tc := range []struct {
		name   string
		status int
		err    error
		want   bool
	}{
		// THE REPORTED BUG: answered with HTTP 200, but client-caused -> WARN.
		{"quota at 200", http.StatusOK, quota, true},
		{"quota at 403", http.StatusForbidden, quota, true},
		// THE MIRROR BUG: middleware forces 401 even for a DB outage -> must be ERROR.
		{"server fault at 401", http.StatusUnauthorized, serverFault, false},
		{"server fault at 500", http.StatusInternalServerError, serverFault, false},
		// Unmarked errors keep the historical status-derived behaviour exactly.
		{"unmarked at 200", http.StatusOK, unmarked, false},
		{"unmarked at 404", http.StatusNotFound, unmarked, true},
		{"unmarked at 500", http.StatusInternalServerError, unmarked, false},
		{"nil at 500", http.StatusInternalServerError, nil, false},
		{"config at 503", http.StatusServiceUnavailable, ConfigErr(errors.New("no channel")), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, LogAsWarn(tc.status, tc.err))
		})
	}
}

func TestKind_StringIsStableAndLabelSafe(t *testing.T) {
	require.Equal(t, "insufficient_quota", InsufficientQuota.String())
	require.Equal(t, "unknown", Unknown.String())
	require.Equal(t, "server", Server.String())

	// Bounded, no user data: safe as a metric label.
	seen := map[string]bool{}
	for k := Unknown; k <= Server; k++ {
		s := k.String()
		require.NotEmpty(t, s)
		require.False(t, seen[s], "duplicate kind name %q", s)
		seen[s] = true
	}
	require.Len(t, seen, int(Server)+1)
}

func TestKind_HTTPStatus(t *testing.T) {
	require.Equal(t, http.StatusForbidden, InsufficientQuota.HTTPStatus())
	require.Equal(t, http.StatusBadRequest, InvalidRequest.HTTPStatus())
	require.Equal(t, http.StatusInternalServerError, Unknown.HTTPStatus())
	require.Equal(t, http.StatusInternalServerError, Server.HTTPStatus())
}
