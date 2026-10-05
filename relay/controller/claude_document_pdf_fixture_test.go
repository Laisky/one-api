package controller

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// claudeDocumentQuotePDF constructs a valid one-page PDF with an identical
// image/text page under either raw or Flate-compressed transport encoding.
// Every xref offset and stream length is derived from the actual bytes.
func claudeDocumentQuotePDF(t *testing.T, compressed bool) string {
	t.Helper()
	pixels := bytes.Repeat([]byte{0x88, 0x99, 0xaa}, 512*512)
	image := pixels
	filter := ""
	if compressed {
		var encoded bytes.Buffer
		writer := zlib.NewWriter(&encoded)
		_, err := writer.Write(pixels)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		image = encoded.Bytes()
		filter = "/Filter /FlateDecode "
	}
	page := []byte("q 256 0 0 256 72 400 cm /Im0 Do Q\nBT /F1 12 Tf 72 350 Td (Synthetic document quote fixture) Tj ET\n")
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Im0 4 0 R >> /Font << /F1 5 0 R >> >> /Contents 6 0 R >>"),
		append(append([]byte(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 512 /Height 512 /ColorSpace /DeviceRGB /BitsPerComponent 8 %s/Length %d >>\nstream\n", filter, len(image))), image...), []byte("\nendstream")...),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
		append(append([]byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(page))), page...), []byte("endstream")...),
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", i+1)
		pdf.Write(object)
		pdf.WriteString("\nendobj\n")
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return base64.StdEncoding.EncodeToString(pdf.Bytes())
}
