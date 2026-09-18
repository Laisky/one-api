package gemini

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/realtime"
	"github.com/Laisky/one-api/relay/relaymode"
)

// ErrLiveProtocol identifies a rejected client frame without including its data.
var ErrLiveProtocol = errors.New("invalid Gemini Live protocol")

// IsLiveChannel reports whether this channel uses the Gemini Developer Live API.
// Parameters: channel is the configured channel type. Returns: native support.
// Vertex has a separate authentication, endpoint and price contract; do not
// silently use a Developer API key or Developer prices for a Vertex session.
func IsLiveChannel(channel int) bool {
	return channel == channeltype.Gemini || channel == channeltype.GeminiOpenAICompatible
}

// LiveRequestURL validates the selected channel/model and constructs a key-free
// upstream URL. Parameters: m is authenticated, mapped request metadata.
// Returns: a WebSocket URL or a configuration error before any network request.
func LiveRequestURL(m *meta.Meta) (string, error) {
	if m == nil || m.Mode != relaymode.Realtime || !IsLiveChannel(m.ChannelType) || !realtime.IsGeminiLiveModel(m.ActualModelName) {
		return "", errors.Wrap(ErrLiveProtocol, "unsupported Gemini Live channel or model")
	}
	version := m.Config.APIVersion
	if version == "" {
		version = "v1alpha"
	}
	if version != "v1alpha" && version != "v1beta" {
		return "", errors.Wrap(ErrLiveProtocol, "unsupported Gemini Live API version")
	}
	base := m.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	override := m.UpstreamEndpointURLOverride()
	if override != "" {
		base = override
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.Wrap(ErrLiveProtocol, "invalid Gemini Live endpoint")
	}
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return "", errors.Wrap(ErrLiveProtocol, "Gemini Live requires TLS")
		}
		u.Scheme = "ws"
	default:
		return "", errors.Wrap(ErrLiveProtocol, "invalid Gemini Live scheme")
	}
	if override == "" {
		prefix := strings.TrimSuffix(u.Path, "/")
		for _, suffix := range []string{"/v1beta/openai", "/v1alpha/openai", "/v1beta", "/v1alpha", "/v1"} {
			if strings.HasSuffix(prefix, suffix) {
				prefix = strings.TrimSuffix(prefix, suffix)
				break
			}
		}
		u.Path = prefix + "/ws/google.ai.generativelanguage." + version + ".GenerativeService.BidiGenerateContent"
		u.RawPath = ""
	}
	return u.String(), nil
}

// prepareLiveSetup validates the first client frame and pins the model to the
// authenticated channel mapping. Parameters: data is JSON, actual is the billed
// model and original is its public alias. Returns: the rewritten setup frame.
// Nested native configuration is preserved; this is not an OpenAI event bridge.
func prepareLiveSetup(data []byte, actual, original string) ([]byte, error) {
	root, err := liveObject(data)
	if err != nil {
		return nil, err
	}
	if len(root) != 1 || root["setup"] == nil {
		return nil, errors.Wrap(ErrLiveProtocol, "first frame must be setup")
	}
	setup, err := liveObject(root["setup"])
	if err != nil {
		return nil, err
	}
	for key := range setup {
		switch key {
		case "model", "generationConfig", "systemInstruction", "tools", "realtimeInputConfig", "contextWindowCompression", "inputAudioTranscription", "outputAudioTranscription", "proactivity", "historyConfig":
		case "sessionResumption":
			return nil, errors.Wrap(ErrLiveProtocol, "session resumption is not enabled; reconnect with a fresh session")
		default:
			return nil, errors.Wrap(ErrLiveProtocol, "unsupported setup field")
		}
	}
	var requested string
	if raw := setup["model"]; raw != nil {
		if err := json.Unmarshal(raw, &requested); err != nil {
			return nil, errors.Wrap(ErrLiveProtocol, "invalid setup model")
		}
	}
	requested = strings.TrimPrefix(requested, "models/")
	if requested != "" && requested != actual && requested != original {
		return nil, errors.Wrap(ErrLiveProtocol, "setup model differs from authenticated model")
	}
	setup["model"], _ = json.Marshal("models/" + actual)
	generation := map[string]json.RawMessage{}
	if raw := setup["generationConfig"]; raw != nil {
		generation, err = liveObject(raw)
		if err != nil {
			return nil, err
		}
	}
	for key := range generation {
		switch key {
		case "responseModalities", "thinkingConfig", "speechConfig", "maxOutputTokens", "temperature", "topP", "topK", "seed":
		default:
			return nil, errors.Wrap(ErrLiveProtocol, "unsupported Live generation field")
		}
	}
	if raw := generation["responseModalities"]; raw != nil {
		var modalities []string
		if err := json.Unmarshal(raw, &modalities); err != nil || len(modalities) != 1 || modalities[0] != "AUDIO" {
			return nil, errors.Wrap(ErrLiveProtocol, "Live requires AUDIO response modality; enable outputAudioTranscription for text")
		}
	} else {
		generation["responseModalities"] = json.RawMessage(`["AUDIO"]`)
	}
	if raw := generation["thinkingConfig"]; raw != nil {
		if actual != "gemini-3.8-live-extended-thinking" {
			return nil, errors.Wrap(ErrLiveProtocol, "ordinary Live has automatic, not configurable, thinking")
		}
		thinking, err := liveObject(raw)
		if err != nil {
			return nil, err
		}
		for key := range thinking {
			if key != "thinkingLevel" {
				return nil, errors.Wrap(ErrLiveProtocol, "unsupported Live thinking setting")
			}
		}
		var level string
		if err := json.Unmarshal(thinking["thinkingLevel"], &level); err != nil {
			return nil, errors.Wrap(ErrLiveProtocol, "invalid Live thinking level")
		}
		level = strings.ToUpper(level)
		switch level {
		case "LOW", "MEDIUM", "HIGH":
		default:
			return nil, errors.Wrap(ErrLiveProtocol, "unsupported Live thinking level")
		}
		thinking["thinkingLevel"], _ = json.Marshal(level)
		generation["thinkingConfig"], err = json.Marshal(thinking)
		if err != nil {
			return nil, errors.Wrap(err, "encode Live thinking config")
		}
	}
	if raw := setup["tools"]; raw != nil {
		setup["tools"], err = prepareLiveTools(raw, actual)
		if err != nil {
			return nil, err
		}
	}
	setup["generationConfig"], err = json.Marshal(generation)
	if err != nil {
		return nil, errors.Wrap(err, "encode Live generation config")
	}
	root["setup"], err = json.Marshal(setup)
	if err != nil {
		return nil, errors.Wrap(err, "encode Live setup")
	}
	result, err := json.Marshal(root)
	return result, errors.Wrap(err, "encode Live client frame")
}

// liveObject validates one bounded JSON object. Parameters: data is a native
// frame or subobject. Returns: raw values, preserving numbers and nested schemas.
func liveObject(data []byte) (map[string]json.RawMessage, error) {
	if err := realtime.ValidateGeminiJSON(data); err != nil {
		return nil, errors.Wrap(ErrLiveProtocol, "malformed or ambiguous JSON")
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil || result == nil {
		return nil, errors.Wrap(ErrLiveProtocol, "expected a JSON object")
	}
	return result, nil
}

// validateLiveClientFrame accepts exactly one native client operation after
// setupComplete. Parameters: data is the frame. Returns: whether work was sent
// and an error on setup mutation, server-shaped usage, or an invalid envelope.
func validateLiveClientFrame(data []byte) (bool, error) {
	root, err := liveObject(data)
	if err != nil {
		return false, err
	}
	if len(root) != 1 {
		return false, errors.Wrap(ErrLiveProtocol, "expected one Live client operation")
	}
	for kind, raw := range root {
		value, err := liveObject(raw)
		if err != nil {
			return false, err
		}
		switch kind {
		case "realtimeInput":
			for key := range value {
				switch key {
				case "audio", "video", "text", "mediaChunks", "audioStreamEnd", "activityStart", "activityEnd":
				default:
					return false, errors.Wrap(ErrLiveProtocol, "unsupported realtime input field")
				}
			}
			return value["audio"] != nil || value["video"] != nil || value["text"] != nil || value["mediaChunks"] != nil, nil
		case "clientContent":
			for key := range value {
				if key != "turns" && key != "turnComplete" {
					return false, errors.Wrap(ErrLiveProtocol, "unsupported client content field")
				}
			}
			return value["turns"] != nil, nil
		case "toolResponse":
			if len(value) != 1 || value["functionResponses"] == nil {
				return false, errors.Wrap(ErrLiveProtocol, "invalid tool response")
			}
			return true, nil
		default:
			return false, errors.Wrap(ErrLiveProtocol, "unsupported client operation or repeated setup")
		}
	}
	return false, errors.WithStack(ErrLiveProtocol)
}
