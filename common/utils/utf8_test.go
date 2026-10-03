package utils

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// TestToValidUTF8 covers the replacement contract: valid input is returned
// unchanged (including non-ASCII), and every run of invalid bytes collapses to
// a single U+FFFD.
func TestToValidUTF8(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"", "/api/status", "/模型/é", "a\x00b"} {
		require.Equal(t, valid, ToValidUTF8(valid))
	}
	require.Equal(t, "/�", ToValidUTF8("/\xc0"))
	require.Equal(t, "a�b", ToValidUTF8("a\xff\xfeb"))
	require.Equal(t, "�x�", ToValidUTF8("\xc0x\xc1\xc2"))
	require.True(t, utf8.ValidString(ToValidUTF8("\xed\xa0\x80"))) // lone surrogate encoding
}

// TestTruncateUTF8 covers the rune-boundary cut: ASCII is cut exactly at the
// budget, multi-byte input is cut before the split rune, and short input plus
// degenerate budgets are handled.
func TestTruncateUTF8(t *testing.T) {
	t.Parallel()

	require.Equal(t, "abc", TruncateUTF8("abc", 3))
	require.Equal(t, "abc", TruncateUTF8("abc", 10))
	require.Equal(t, "ab", TruncateUTF8("abcd", 2))
	require.Equal(t, "", TruncateUTF8("abc", 0))
	require.Equal(t, "", TruncateUTF8("abc", -1))

	s := "/" + strings.Repeat("é", 10) // 21 bytes
	got := TruncateUTF8(s, 4)          // byte 4 is the second half of an "é"
	require.Equal(t, "/é", got)
	require.True(t, utf8.ValidString(got))

	cjk := strings.Repeat("模", 5) // 15 bytes
	require.Equal(t, "模", TruncateUTF8(cjk, 5))
	require.Equal(t, "模模", TruncateUTF8(cjk, 6))
}
