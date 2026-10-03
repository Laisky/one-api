package identity

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// theExampleMessage is the exact error text an operator reported as unreadable.
// It must survive tagging byte-for-byte: it is echoed to API clients, matched by
// strings.Contains classifiers, and asserted literally by other tests.
const theExampleMessage = "insufficient user quota: required=50, available=0, userId=175, tokenId=257"

func TestTag_PreservesErrorTextByteIdentically(t *testing.T) {
	orig := errors.Errorf("insufficient user quota: required=%d, available=%d, userId=%d, tokenId=%d",
		50, 0, 175, 257)
	require.Equal(t, theExampleMessage, orig.Error())

	tagged := Tag(orig,
		NewUserRef(175, "user-uuid", "alice"),
		NewTokenRef(257, "token-uuid", "laptop-cli"))

	require.Equal(t, theExampleMessage, tagged.Error())
	require.Equal(t, orig.Error(), tagged.Error())
}

func TestTag_PreservesStackFormatting(t *testing.T) {
	orig := errors.Errorf("insufficient token quota: required=%d, available=%d, tokenId=%d", 100, 0, 1)
	tagged := Tag(orig, NewTokenRef(1, "uuid", "name"))

	require.Equal(t, fmt.Sprintf("%+v", orig), fmt.Sprintf("%+v", tagged))
	require.Equal(t, fmt.Sprintf("%v", orig), fmt.Sprintf("%v", tagged))
	require.Equal(t, fmt.Sprintf("%s", orig), fmt.Sprintf("%s", tagged))
	require.Equal(t, fmt.Sprintf("%q", orig), fmt.Sprintf("%q", tagged))
	// The stack must still be printed, i.e. %+v is richer than the bare message.
	require.Contains(t, fmt.Sprintf("%+v", tagged), "identity.TestTag_PreservesStackFormatting")
}

func TestTag_TransparentToIsAs(t *testing.T) {
	sentinel := stderrors.New("record not found")
	wrapped := errors.Wrap(sentinel, "load token")
	tagged := Tag(wrapped, NewTokenRef(7, "uuid", "name"))

	require.True(t, errors.Is(tagged, sentinel))
	require.True(t, stderrors.Is(tagged, sentinel))

	// And it stays transparent when further wrapped by the repo's error helper.
	outer := errors.Wrapf(tagged, "pre-consume token quota")
	require.True(t, errors.Is(outer, sentinel))
}

// TestTag_TransparentToStringsContains guards the classifiers in
// controller/relay_error.go and middleware/utils.go, which decide retry behaviour,
// channel suspension and WARN-vs-ERROR from the message text.
func TestTag_TransparentToStringsContains(t *testing.T) {
	for _, probe := range []string{
		"insufficient user quota",
		"insufficient token quota",
		"token not found for key:",
		"No available channels for Model",
	} {
		base := errors.Errorf("%s foo", probe)
		tagged := Tag(base, NewUserRef(1, "u", "n"))
		outer := errors.Wrapf(tagged, "pre-consume phase of immediate consume")

		require.True(t, strings.Contains(tagged.Error(), probe), probe)
		require.True(t, strings.Contains(outer.Error(), probe), probe)
	}
}

func TestTag_NilAndZeroRefs(t *testing.T) {
	require.Nil(t, Tag(nil, NewUserRef(1, "u", "n")))

	base := errors.New("boom")
	require.Equal(t, base, Tag(base))
	require.Equal(t, base, Tag(base, UserRef{}))
	require.Equal(t, base, Tag(base, UserRef{}, TokenRef{}, ChannelRef{}))
}

func TestFields_WalksWrapChainAndDedupes(t *testing.T) {
	inner := Tag(errors.New("inner"), NewTokenRef(257, "token-uuid", "laptop-cli"))
	middle := errors.Wrapf(inner, "pre-consume token quota")
	outer := Tag(middle, NewUserRef(175, "user-uuid", "alice"))

	got := fieldMap(Fields(outer))
	require.Equal(t, int64(175), got["user_id"])
	require.Equal(t, "alice", got["username"])
	require.Equal(t, int64(257), got["token_id"])
	require.Equal(t, "laptop-cli", got["token_name"])

	// Outermost wins, and each key appears once.
	shadowed := Tag(outer, NewUserRef(999, "other-uuid", "bob"))
	fields := Fields(shadowed)
	require.Equal(t, 1, keyCount(fields, "user_id"))
	require.Equal(t, int64(999), fieldMap(fields)["user_id"])

	require.Nil(t, Fields(nil))
	require.Nil(t, Fields(errors.New("untagged")))
}

func TestSetFrom_RecoversValues(t *testing.T) {
	err := Tag(errors.Wrap(
		Tag(errors.New("inner"), NewChannelRef(42, "c-uuid", "openai-main")),
		"relay failed"), NewUserRef(175, "u-uuid", "alice"))

	got := SetFrom(err)
	require.Equal(t, 175, got.User.ID)
	require.Equal(t, "openai-main", got.Channel.Name)
	require.True(t, got.Token.IsZero())

	require.True(t, SetFrom(nil).IsZero())
}
