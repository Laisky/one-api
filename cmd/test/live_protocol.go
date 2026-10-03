package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
)

// liveTurn summarizes one server turn without retaining audio payloads.
type liveTurn struct {
	audioChunks           int
	frames                int
	transcript            string
	turnComplete          bool
	interactionIdle       bool
	interactionInProgress bool
	usage                 map[string]any
	usageJSON             string
}

// readLiveTurn consumes frames until a receipt and terminal boundary arrive.
// Parameters: conn is an acknowledged socket and timeout bounds the turn.
// Returns: a summary or an error. IN_PROGRESS to IDLE is independently terminal;
// a spoken turnComplete while background work remains active is not terminal.
func readLiveTurn(conn *websocket.Conn, timeout time.Duration) (liveTurn, error) {
	var turn liveTurn
	deadline := time.Now().Add(timeout)
	for turn.frames < liveMaxFramesPerTurn {
		read := time.Now().Add(liveReadDeadline)
		if read.After(deadline) {
			read = deadline
		}
		if err := conn.SetReadDeadline(read); err != nil {
			return turn, errors.Wrap(err, "set read deadline")
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return turn, errors.Errorf("session closed mid-turn with %d (%s)", closeErr.Code, closeErr.Text)
			}
			return turn, errors.Wrap(err, "read live frame")
		}
		turn.frames++
		var event struct {
			Usage             map[string]any `json:"usageMetadata"`
			InteractionStatus string         `json:"interactionStatus"`
			ServerContent     *struct {
				ModelTurn *struct {
					Parts []struct {
						InlineData *struct {
							MimeType string `json:"mimeType"`
						} `json:"inlineData"`
					} `json:"parts"`
				} `json:"modelTurn"`
				OutputTranscription *struct {
					Text string `json:"text"`
				} `json:"outputTranscription"`
				TurnComplete      bool   `json:"turnComplete"`
				InteractionStatus string `json:"interactionStatus"`
			} `json:"serverContent"`
		}
		if err := json.Unmarshal(msg, &event); err != nil {
			return turn, errors.Wrap(err, "decode live frame")
		}
		if event.Usage != nil {
			turn.usage = event.Usage
			turn.usageJSON = truncateString(string(msg), 512)
		}
		if event.ServerContent != nil && event.ServerContent.ModelTurn != nil {
			for _, part := range event.ServerContent.ModelTurn.Parts {
				if part.InlineData != nil {
					turn.audioChunks++
				}
			}
		}
		if event.ServerContent != nil && event.ServerContent.OutputTranscription != nil {
			turn.transcript = appendLiveTranscript(turn.transcript, event.ServerContent.OutputTranscription.Text)
		}
		if event.ServerContent != nil && event.ServerContent.TurnComplete {
			turn.turnComplete = true
		}
		status := event.InteractionStatus
		if event.ServerContent != nil && event.ServerContent.InteractionStatus != "" {
			status = event.ServerContent.InteractionStatus
		}
		switch strings.ToUpper(strings.TrimSpace(status)) {
		case "IN_PROGRESS":
			turn.interactionInProgress = true
			turn.interactionIdle = false
		case "IDLE":
			if turn.interactionInProgress {
				turn.interactionIdle = true
			}
		}
		if turn.usage != nil && ((turn.turnComplete && !turn.interactionInProgress) || turn.interactionIdle) {
			return turn, nil
		}
	}
	return turn, errors.New("turn exceeded the frame budget without a receipt")
}

// appendLiveTranscript retains a bounded, rune-safe diagnostic transcript.
// Parameters: transcript is the preview and text the new fragment. Returns:
// a preview no larger than maxLiveTranscriptBytes.
func appendLiveTranscript(transcript, text string) string {
	remaining := maxLiveTranscriptBytes - len(transcript)
	if remaining <= 0 || text == "" {
		return transcript
	}
	if len(text) <= remaining {
		return transcript + text
	}
	for _, r := range text {
		fragment := string(r)
		if len(fragment) > remaining {
			break
		}
		transcript += fragment
		remaining -= len(fragment)
	}
	return transcript
}

// liveHandshake sends setup and waits for acknowledgement. Parameters: conn,
// setup and timeout describe the handshake. Returns: a setup or transport error.
func liveHandshake(conn *websocket.Conn, setup map[string]any, timeout time.Duration) error {
	if err := writeLiveJSON(conn, setup); err != nil {
		return errors.Wrap(err, "send setup")
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return errors.Wrap(err, "set read deadline")
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) {
			return errors.Errorf("setup rejected with close %d (%s)", closeErr.Code, closeErr.Text)
		}
		return errors.Wrap(err, "read setup acknowledgement")
	}
	if !isLiveSetupComplete(msg) {
		return errors.Errorf("first server frame is not setupComplete: %s", truncateString(string(msg), 256))
	}
	return nil
}

// isLiveSetupComplete reports whether a frame acknowledges setup. Parameters:
// msg is one native frame. Returns: whether setupComplete exists and is non-null.
func isLiveSetupComplete(msg []byte) bool {
	var frame map[string]json.RawMessage
	if err := json.Unmarshal(msg, &frame); err != nil {
		return false
	}
	raw, exists := frame["setupComplete"]
	return exists && string(raw) != "null"
}

// writeLiveJSON sends one native JSON frame. Parameters: conn is the socket and
// frame the operation. Returns: a wrapped encoding or transport error.
func writeLiveJSON(conn *websocket.Conn, frame map[string]any) error {
	payload, err := json.Marshal(frame)
	if err != nil {
		return errors.Wrap(err, "encode live frame")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return errors.Wrap(err, "set write deadline")
	}
	return errors.Wrap(conn.WriteMessage(websocket.TextMessage, payload), "write live frame")
}
