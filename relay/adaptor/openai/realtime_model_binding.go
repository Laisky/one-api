package openai

import (
	"bytes"
	"encoding/json"
	"github.com/Laisky/errors/v2"
)

// enforceRealtimeSessionUpdate holds model-bearing updates to the authenticated
// handshake binding. transcription comes from the handshake, not an editable
// frame; ordinary conversations retain their independent auxiliary transcriber.
// frame is the original JSON, boundModel is the upstream model, and originModel
// is its authorized public alias. It returns an unchanged or normalized frame,
// or ErrModelSwitchDenied without forwarding any part of a conflicting update.
func enforceRealtimeSessionUpdate(frame []byte, boundModel, originModel string, transcription bool) ([]byte, error) {
	if len(frame) == 0 || boundModel == "" {
		return frame, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(frame, &raw); err != nil {
		return frame, nil
	}
	var eventType string
	if err := json.Unmarshal(raw["type"], &eventType); err != nil {
		return frame, nil
	}
	switch eventType {
	case "session.update":
	case "transcription_session.update":
		if !transcription {
			return frame, errors.Wrap(ErrModelSwitchDenied, "cannot change a conversation into a transcription session")
		}
	default:
		return frame, nil
	}
	var session map[string]json.RawMessage
	if err := json.Unmarshal(raw["session"], &session); err == nil {
		if value, exists := session["type"]; exists {
			var requested string
			expected := "realtime"
			if transcription {
				expected = "transcription"
			}
			if err := json.Unmarshal(value, &requested); err != nil || requested != expected {
				return frame, errors.Wrap(ErrModelSwitchDenied, "session type does not match the handshake")
			}
		}
	}
	paths := [][]string{{"session", "model"}}
	if transcription {
		paths = append(paths,
			[]string{"session", "audio", "input", "transcription", "model"},
			[]string{"session", "input_audio_transcription", "model"},
			[]string{"input_audio_transcription", "model"})
	}
	changed := false
	for _, path := range paths {
		rewritten, err := enforceBoundRealtimeModelAtPath(raw, path, boundModel, originModel, transcription)
		if err != nil {
			return frame, err
		}
		changed = changed || rewritten
	}
	if !changed {
		return frame, nil
	}
	rewritten, err := json.Marshal(raw)
	if err != nil {
		return frame, errors.Wrap(err, "encode normalized realtime update")
	}
	return rewritten, nil
}

// enforceBoundRealtimeModelAtPath validates one fixed-depth model path in object.
// A matching public alias is normalized, preserving unrelated raw JSON numbers.
// strictNull rejects null model selection for transcription; a normal session's
// historical null model behavior is unchanged. It returns whether object changed
// and a wrapped policy or encoding error. Missing configuration is left alone.
func enforceBoundRealtimeModelAtPath(object map[string]json.RawMessage, path []string, boundModel, originModel string, strictNull bool) (bool, error) {
	value, exists := object[path[0]]
	if !exists {
		return false, nil
	}
	if len(path) > 1 {
		var child map[string]json.RawMessage
		if err := json.Unmarshal(value, &child); err != nil || child == nil {
			return false, nil
		}
		changed, err := enforceBoundRealtimeModelAtPath(child, path[1:], boundModel, originModel, strictNull)
		if err != nil {
			return false, err
		}
		if !changed {
			return false, nil
		}
		encoded, err := json.Marshal(child)
		if err != nil {
			return false, errors.Wrap(err, "encode normalized realtime configuration")
		}
		object[path[0]] = encoded
		return true, nil
	}
	if !strictNull && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return false, nil
	}
	var requested string
	if err := json.Unmarshal(value, &requested); err != nil || requested == "" {
		return false, errors.Wrap(ErrModelSwitchDenied, "realtime model must be a non-empty string")
	}
	if requested == boundModel {
		return false, nil
	}
	if originModel == "" || requested != originModel {
		return false, errors.Wrap(ErrModelSwitchDenied, "realtime model differs from the authenticated binding")
	}
	encoded, err := json.Marshal(boundModel)
	if err != nil {
		return false, errors.Wrap(err, "encode bound realtime model")
	}
	object[path[0]] = encoded
	return true, nil
}
