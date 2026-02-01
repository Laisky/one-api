package helper

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfigureScannerBuffer_AllowsLargeToken verifies the scanner accepts tokens larger than the default limit.
func TestConfigureScannerBuffer_AllowsLargeToken(t *testing.T) {
	t.Parallel()
	largeToken := strings.Repeat("a", 256*1024)
	input := largeToken + "\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	ConfigureScannerBuffer(scanner)
	scanner.Split(bufio.ScanLines)

	require.True(t, scanner.Scan())
	require.Equal(t, largeToken, scanner.Text())
	require.NoError(t, scanner.Err())
}
