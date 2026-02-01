package helper

import "bufio"

// DefaultScannerInitialBufferSize defines the initial buffer capacity for scanners.
const DefaultScannerInitialBufferSize = 64 * 1024

// DefaultScannerMaxTokenSize defines the maximum token size supported by scanners.
const DefaultScannerMaxTokenSize = 32 * 1024 * 1024

// ConfigureScannerBuffer configures the scanner buffer sizes to handle large tokens.
// It is safe to call multiple times for the same scanner.
func ConfigureScannerBuffer(scanner *bufio.Scanner) {
	if scanner == nil {
		return
	}
	buffer := make([]byte, DefaultScannerInitialBufferSize)
	scanner.Buffer(buffer, DefaultScannerMaxTokenSize)
}
