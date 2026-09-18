package typesafe

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Request is the native System One evaluation payload, not a chat request.
// RawMessage preserves structured descriptions, nulls and large JSON integers.
type Request struct {
	Model     string              `json:"model"`
	State     json.RawMessage     `json:"state"`
	Questions map[string]Question `json:"questions"`
}

// Question describes one of TypeSafe's typed evaluations. The provider's own
// discriminator error enumerates noul, choice, score and bounding_box.
type Question struct {
	Type         string          `json:"type"`
	Instructions json.RawMessage `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

// DecodeRequest validates the documented native shape without coercing JSON data.
func DecodeRequest(body []byte) (*Request, error) {
	fields, err := decodeObject(body)
	if err != nil {
		return nil, errors.Wrap(err, "decode System One request")
	}
	if err := allowFields(fields, "model", "state", "questions"); err != nil {
		return nil, err
	}
	var request Request
	if err := json.Unmarshal(fields["model"], &request.Model); err != nil || strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("model must be a nonempty string")
	}
	request.State = fields["state"]
	if !isDescription(request.State, false) {
		return nil, errors.New("state must be a string, object or array")
	}
	questions, err := decodeObject(fields["questions"])
	if err != nil || len(questions) == 0 {
		return nil, errors.New("questions must be a nonempty object")
	}
	request.Questions = make(map[string]Question, len(questions))
	for id, raw := range questions {
		// Verified upstream: an empty key is rejected unconditionally with
		// 400 {"detail":"Question key cannot be empty."}, and an answer could
		// never be addressed by it.
		if id == "" {
			return nil, errors.New("question IDs must not be empty")
		}
		question, err := decodeQuestion(raw)
		if err != nil {
			return nil, errors.Wrapf(err, "question %q", id)
		}
		request.Questions[id] = question
	}
	return &request, nil
}

// decodeQuestion checks question-specific criteria while retaining native JSON.
func decodeQuestion(raw json.RawMessage) (Question, error) {
	var question Question
	fields, err := decodeObject(raw)
	if err != nil {
		return question, err
	}
	if err := allowFields(fields, "type", "instructions", "criteria"); err != nil {
		return question, err
	}
	if err := json.Unmarshal(fields["type"], &question.Type); err != nil {
		return question, errors.New("type must be noul, choice, score or bounding_box")
	}
	question.Instructions, question.Criteria = fields["instructions"], fields["criteria"]
	// The advanced structure reference explicitly permits null instructions, and
	// the service accepts a null value for every documented primitive. Whether
	// null alone is sufficient is a content rule the service owns: noul rejects
	// a question that carries neither instructions nor criteria, and that
	// rejection now refunds rather than charging the reservation.
	if question.Type != "bounding_box" && !isDescription(question.Instructions, true) {
		return question, errors.New("instructions must be present and contain a string, object, array or null")
	}
	switch question.Type {
	case "noul":
		if len(question.Criteria) == 0 || bytes.Equal(bytes.TrimSpace(question.Criteria), []byte("null")) {
			return question, nil
		}
		criteria, err := decodeObject(question.Criteria)
		if err != nil {
			return question, errors.Wrap(err, "noul criteria must be an object")
		}
		if err := allowFields(criteria, "true", "false"); err != nil {
			return question, err
		}
		for _, description := range criteria {
			if !isDescription(description, true) {
				return question, errors.New("invalid noul criterion description")
			}
		}
	case "choice":
		criteria, err := decodeObject(question.Criteria)
		if err != nil || len(criteria) == 0 {
			return question, errors.New("choice criteria must be a nonempty object")
		}
		for _, description := range criteria {
			if !isDescription(description, true) {
				return question, errors.New("invalid choice option description")
			}
		}
	case "score":
		// The reference recommends two to ten levels, but the service only
		// enforces a nonempty array (a single level is answered with score 0).
		// Magnitude caps are the provider's to enforce and to change; the
		// gateway validates shape so that a future cap change cannot make it
		// reject requests the service would have accepted.
		var levels []json.RawMessage
		if err := json.Unmarshal(question.Criteria, &levels); err != nil || len(levels) == 0 {
			return question, errors.New("score criteria must be a nonempty array of ordered levels")
		}
		for _, description := range levels {
			if !isDescription(description, true) {
				return question, errors.New("invalid score level description")
			}
		}
	case "bounding_box":
		// Advertised by the service's discriminator but absent from the public
		// reference and gated per organization, so its criteria schema is not
		// knowable here. The envelope is validated and the payload forwarded
		// verbatim; organizations without the feature receive the provider's
		// own 400, which refunds the admission reservation.
		for _, field := range []json.RawMessage{question.Instructions, question.Criteria} {
			if len(field) > 0 && !isDescription(field, true) {
				return question, errors.New("bounding_box fields must contain a string, object, array or null")
			}
		}
	default:
		return question, errors.New("type must be noul, choice, score or bounding_box")
	}
	return question, nil
}

// isDescription checks the outer shape only; nested JSON remains opaque.
func isDescription(raw json.RawMessage, nullable bool) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	switch raw[0] {
	case '"', '{', '[':
		return true
	case 'n':
		return nullable && bytes.Equal(raw, []byte("null"))
	default:
		return false
	}
}

// decodeObject rejects duplicate envelope keys and trailing JSON values.
// Nested state/description objects are intentionally not rewritten or traversed.
func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("expected a JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.Wrap(err, "read object key")
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("object key must be a string")
		}
		if _, exists := fields[key]; exists {
			return nil, errors.Errorf("duplicate field %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.Wrap(err, "read object value")
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.Wrap(err, "close object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after JSON object")
	}
	return fields, nil
}

// allowFields rejects unsupported envelope parameters instead of dropping them.
func allowFields(fields map[string]json.RawMessage, allowed ...string) error {
	for key := range fields {
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("unsupported field %q", key)
		}
	}
	return nil
}
