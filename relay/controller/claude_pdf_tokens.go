package controller

import (
	"encoding/base64"
	"io"
	"math"
	"math/bits"

	"github.com/Laisky/errors/v2"
)

// claudePDFTokensForBytes rounds a nonnegative decoded byte count up at the
// configured positive tokens-per-KiB rate, returning unrepresentable estimates
// as errors. Representable estimates have no policy ceiling.
func claudePDFTokensForBytes(decodedBytes, tokensPerKiB int) (int, error) {
	if decodedBytes < 0 || tokensPerKiB < 1 {
		return 0, errors.New("invalid native PDF byte count or token rate")
	}
	high, low := bits.Mul64(uint64(decodedBytes), uint64(tokensPerKiB))
	if high >= 1024 {
		return 0, errors.New("native PDF token estimate exceeds integer range")
	}
	whole, remainder := bits.Div64(high, low, 1024)
	if whole > uint64(math.MaxInt) || (whole == uint64(math.MaxInt) && remainder != 0) {
		return 0, errors.New("native PDF token estimate exceeds integer range")
	}
	if remainder != 0 {
		whole++
	}
	return int(whole), nil
}

// claudePDFDecodedBytes validates and counts base64 bytes with bounded decoder
// storage. ASCII transport whitespace is ignored for the estimate; the original
// provider payload is unchanged and no PDF parser or decompressor is invoked.
func claudePDFDecodedBytes(data string) (int, error) {
	reader := &claudePDFBase64Reader{data: data}
	decoded, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, reader))
	if err != nil {
		return 0, errors.Wrap(err, "decode native PDF source for quota estimate")
	}
	if decoded == 0 || decoded > int64(math.MaxInt) {
		return 0, errors.New("native PDF source has empty or unrepresentable decoded size")
	}
	return int(decoded), nil
}

// claudePDFBase64Reader streams an existing base64 string while skipping ASCII
// transport whitespace, without copying or retaining a second document buffer.
type claudePDFBase64Reader struct {
	data   string
	offset int
}

// Read fills buffer with non-whitespace encoded bytes and returns EOF when all
// input has been consumed. It preserves every non-whitespace byte for decoding.
func (reader *claudePDFBase64Reader) Read(buffer []byte) (int, error) {
	written := 0
	for reader.offset < len(reader.data) && written < len(buffer) {
		value := reader.data[reader.offset]
		reader.offset++
		switch value {
		case ' ', '\t', '\r', '\n':
			continue
		}
		buffer[written] = value
		written++
	}
	if written == 0 && reader.offset == len(reader.data) {
		return 0, io.EOF
	}
	return written, nil
}
