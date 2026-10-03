package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"mime"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// decodeAudio validates the native encoding and returns mono PCM16LE at 24 kHz.
func decodeAudio(data []byte, contentType string) ([]byte, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errors.Wrap(err, "invalid Gemini speech MIME type")
	}
	if rate := params["rate"]; rate != "" && rate != "24000" {
		return nil, errors.New("unexpected speech sample rate")
	}
	if channels := params["channels"]; channels != "" && channels != "1" {
		return nil, errors.New("unexpected speech channel count")
	}
	switch strings.ToLower(mediaType) {
	case "audio/wav", "audio/x-wav":
		return decodeWAV(data)
	case "audio/l16", "audio/pcm":
		if len(data) == 0 || len(data)%2 != 0 || len(data) > MaxAudioBytes || bytes.HasPrefix(data, []byte("RIFF")) {
			return nil, errors.New("invalid headerless PCM speech payload")
		}
		return data, nil
	default:
		return nil, errors.New("unexpected Gemini speech encoding")
	}
}

// decodeWAV walks RIFF chunks instead of assuming every WAV header is 44 bytes.
func decodeWAV(data []byte) ([]byte, error) {
	if len(data) < 44 || len(data) > MaxAudioBytes+4096 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return nil, errors.New("invalid or truncated WAV container")
	}
	validFormat := false
	var pcm []byte
	for pos := 12; pos < len(data); {
		if len(data)-pos < 8 {
			return nil, errors.New("truncated WAV chunk header")
		}
		name := string(data[pos : pos+4])
		size := uint64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8
		if size > uint64(len(data)-pos) {
			return nil, errors.New("truncated WAV chunk")
		}
		chunk := data[pos : pos+int(size)]
		switch name {
		case "fmt ":
			if validFormat || len(chunk) < 16 || binary.LittleEndian.Uint16(chunk) != 1 || binary.LittleEndian.Uint16(chunk[2:]) != 1 || binary.LittleEndian.Uint32(chunk[4:]) != sampleRate || binary.LittleEndian.Uint32(chunk[8:]) != sampleRate*2 || binary.LittleEndian.Uint16(chunk[12:]) != 2 || binary.LittleEndian.Uint16(chunk[14:]) != 16 {
				return nil, errors.New("WAV must contain mono PCM16LE at 24 kHz")
			}
			validFormat = true
		case "data":
			if pcm != nil || len(chunk) == 0 || len(chunk)%2 != 0 {
				return nil, errors.New("invalid WAV audio data")
			}
			pcm = chunk
		}
		pos += int(size) + int(size%2)
		if pos > len(data) {
			return nil, errors.New("missing WAV chunk padding")
		}
	}
	if !validFormat || len(pcm) == 0 {
		return nil, errors.New("WAV format or audio data is missing")
	}
	return pcm, nil
}

// makeWAV wraps validated raw PCM once with an exact-length RIFF header.
func makeWAV(pcm []byte) []byte {
	out := make([]byte, 44+len(pcm))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(pcm)))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], 1)
	binary.LittleEndian.PutUint32(out[24:], sampleRate)
	binary.LittleEndian.PutUint32(out[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(out[32:], 2)
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(pcm)))
	copy(out[44:], pcm)
	return out
}

// limitedBuffer rejects unbounded encoder output without retaining excessive bytes.
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

// Write appends p only when the configured byte ceiling is respected.
func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("encoded speech exceeds size limit")
	}
	return b.Buffer.Write(p)
}

// encode converts a complete PCM receipt into the requested codec without shell expansion.
// Encoding and speed changes do not alter the billable upstream audio duration.
func (p *Plan) encode(ctx context.Context, pcm []byte) ([]byte, error) {
	if p.Speed == 1 {
		if p.Format == "pcm" {
			return pcm, nil
		}
		if p.Format == "wav" {
			return makeWAV(pcm), nil
		}
	}
	if p.Encoder == "" {
		return nil, errors.New("speech encoder is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-threads", "1", "-f", "s16le", "-ar", "24000", "-ac", "1", "-i", "pipe:0"}
	if p.Speed != 1 {
		speed := p.Speed
		var filters []string
		for speed < 0.5 {
			filters = append(filters, "atempo=0.5")
			speed /= 0.5
		}
		for speed > 2 {
			filters = append(filters, "atempo=2")
			speed /= 2
		}
		filters = append(filters, "atempo="+strconv.FormatFloat(speed, 'f', -1, 64))
		args = append(args, "-af", strings.Join(filters, ","))
	}
	format, codec := p.Format, ""
	switch p.Format {
	case "pcm", "wav":
		format = "s16le"
		codec = "pcm_s16le"
	case "mp3":
		codec = "libmp3lame"
	case "opus":
		format = "ogg"
		codec = "libopus"
	case "aac":
		format = "adts"
		codec = "aac"
	case "flac":
		codec = "flac"
	}
	args = append(args, "-c:a", codec, "-threads", "1", "-f", format, "pipe:1")
	cmd := exec.CommandContext(ctx, p.Encoder, args...)
	cmd.Stdin = bytes.NewReader(pcm)
	output := &limitedBuffer{limit: 128 << 20}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return nil, errors.New("speech audio encoding failed")
	}
	if output.Len() == 0 {
		return nil, errors.New("speech encoder returned empty audio")
	}
	if p.Format == "wav" {
		return makeWAV(output.Bytes()), nil
	}
	return output.Bytes(), nil
}

// mimeType returns the content type of the requested standard speech format.
func (p *Plan) mimeType() string {
	return map[string]string{"pcm": "audio/pcm", "wav": "audio/wav", "mp3": "audio/mpeg", "opus": "audio/ogg", "aac": "audio/aac", "flac": "audio/flac"}[p.Format]
}
