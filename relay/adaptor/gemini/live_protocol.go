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

// IsLiveChannel reports whether a channel uses the Developer API transport.
// Parameters: channel is its configured type. Returns: true for native and
// compatibility Gemini channels; Vertex supplies a distinct OAuth transport.
func IsLiveChannel(channel int) bool {
	return channel == channeltype.Gemini || channel == channeltype.GeminiOpenAICompatible
}

// ValidateLiveModelName validates a configured model ID's syntax, not its release
// stage or upstream entitlement. Parameters: name is the mapped ID. Returns: an
// error only for an empty, oversized, or path-escaping ID. The catalog is not an allowlist.
func ValidateLiveModelName(name string) error {
	if len(name) == 0 || len(name) > 200 || name == "." || name == ".." {
		return errors.Wrap(ErrLiveProtocol, "invalid Live model ID")
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("-_.@", c)) {
			return errors.Wrap(ErrLiveProtocol, "invalid Live model ID")
		}
	}
	return nil
}

// LiveRequestURL constructs the Developer API endpoint. Parameters: m contains
// authorized channel metadata. Returns: a key-free WebSocket URL or a local
// configuration error. Model access is decided by the upstream, not this function.
func LiveRequestURL(m *meta.Meta) (string, error) {
	if m == nil || m.Mode != relaymode.Realtime || !IsLiveChannel(m.ChannelType) {
		return "", errors.Wrap(ErrLiveProtocol, "unsupported Gemini Live channel")
	}
	if err := ValidateLiveModelName(m.ActualModelName); err != nil {
		return "", err
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
	if override := m.UpstreamEndpointURLOverride(); override != "" {
		return ResolveLiveEndpoint(override, "", true)
	}
	return ResolveLiveEndpoint(base, "/ws/google.ai.generativelanguage."+version+".GenerativeService.BidiGenerateContent", false)
}

// ResolveLiveEndpoint validates a credential-free, administrator-owned endpoint.
// Parameters: base is the configured URL, suffix the native RPC path, and exact
// preserves an explicit endpoint override. Returns: a TLS WebSocket URL; plaintext
// is allowed only for literal loopback fixtures. No caller headers or query are used.
func ResolveLiveEndpoint(base, suffix string, exact bool) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
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
	if !exact {
		prefix := strings.TrimSuffix(u.Path, "/")
		for _, ending := range []string{"/v1beta/openai", "/v1alpha/openai", "/v1beta1", "/v1beta", "/v1alpha", "/v1"} {
			if strings.HasSuffix(prefix, ending) {
				prefix = strings.TrimSuffix(prefix, ending)
				break
			}
		}
		u.Path = prefix + suffix
		u.RawPath = ""
	}
	return u.String(), nil
}

// prepareLiveSetup validates setup for the Developer API. Parameters: data is
// JSON, actual is the mapped model and original its public alias. Returns: a
// model-pinned native frame. Vertex uses prepareLiveSetupForResource instead.
func prepareLiveSetup(data []byte, actual, original string) ([]byte, error) {
	return prepareLiveSetupForResource(data, actual, original, "models/"+actual)
}

// prepareLiveSetupForResource validates setup and pins its provider resource.
// Parameters: data is JSON; actual, original, and resource come exclusively from
// authenticated channel metadata. Returns: the rewritten frame. A client cannot
// change the billed model, project, or location through its setup object.
func prepareLiveSetupForResource(data []byte, actual, original, resource string) ([]byte, error) {
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
	if requested != resource {
		requested = strings.TrimPrefix(requested, "models/")
		if requested != "" && requested != actual && requested != original {
			return nil, errors.Wrap(ErrLiveProtocol, "setup model differs from authenticated model")
		}
	}
	setup["model"], _ = json.Marshal(resource)
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
		if err := json.Unmarshal(raw, &modalities); err != nil || len(modalities) != 1 || (modalities[0] != "AUDIO" && modalities[0] != "TEXT") {
			return nil, errors.Wrap(ErrLiveProtocol, "Live requires one AUDIO or TEXT response modality")
		}
		if realtime.IsGeminiLiveModel(actual) && modalities[0] != "AUDIO" {
			return nil, errors.Wrap(ErrLiveProtocol, "Live requires AUDIO response modality; enable outputAudioTranscription for text")
		}
	} else {
		generation["responseModalities"] = json.RawMessage(`["AUDIO"]`)
	}
	if raw := generation["thinkingConfig"]; raw != nil {
		if actual == "gemini-3.8-live" {
			return nil, errors.Wrap(ErrLiveProtocol, "ordinary Live has automatic, not configurable, thinking")
		}
		thinking, err := liveObject(raw)
		if err != nil {
			return nil, err
		}
		if actual == "gemini-3.8-live-extended-thinking" {
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
		}
		// Other configured models retain their provider-native thinking settings.
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
