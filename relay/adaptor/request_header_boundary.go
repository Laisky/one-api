package adaptor

import (
	"net/http"
	"strings"
)

// ForwardableRequestHeaders returns an independent copy of caller headers with
// gateway credentials and hop-by-hop fields removed. Provider authentication
// and administrator-owned custom headers must be applied after this boundary.
func ForwardableRequestHeaders(source http.Header) http.Header {
	blocked := map[string]struct{}{
		"authorization": {}, "x-api-key": {}, "api-key": {},
		"cookie": {}, "cookie2": {}, "proxy-authorization": {},
		"host": {}, "content-length": {}, "accept-encoding": {},
		"connection": {}, "keep-alive": {}, "proxy-connection": {},
		"proxy-authenticate": {}, "te": {}, "trailer": {},
		"transfer-encoding": {}, "upgrade": {},
	}
	for name, values := range source {
		if !strings.EqualFold(name, "Connection") {
			continue
		}
		for _, value := range values {
			for _, token := range strings.Split(value, ",") {
				blocked[strings.ToLower(strings.TrimSpace(token))] = struct{}{}
			}
		}
	}
	out := make(http.Header, len(source))
	for name, values := range source {
		if _, deny := blocked[strings.ToLower(name)]; deny {
			continue
		}
		canonical := http.CanonicalHeaderKey(name)
		if strings.EqualFold(name, "Sec-WebSocket-Protocol") {
			for _, value := range values {
				for _, protocol := range strings.Split(value, ",") {
					protocol = strings.TrimSpace(protocol)
					if protocol == "" || strings.HasPrefix(strings.ToLower(protocol), "openai-insecure-api-key.") {
						continue
					}
					out[canonical] = append(out[canonical], protocol)
				}
			}
			continue
		}
		out[canonical] = append(out[canonical], values...)
	}
	if protocols := out.Values("Sec-WebSocket-Protocol"); len(protocols) > 0 {
		out.Set("Sec-WebSocket-Protocol", strings.Join(protocols, ", "))
	}
	return out
}
