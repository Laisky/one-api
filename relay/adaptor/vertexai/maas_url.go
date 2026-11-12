package vertexai

import (
	"fmt"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/vertexai/maas"
	"github.com/Laisky/one-api/relay/meta"
)

// isManagedOpenModel recognizes exact documented MaaS IDs rather than publisher prefixes.
func isManagedOpenModel(name string) bool {
	_, _, ok := maas.EndpointConfig(name)
	return ok
}

// buildMaaSURL uses each managed model's published region while retaining explicit channel overrides.
func (a *Adaptor) buildMaaSURL(m *meta.Meta) (string, error) {
	host, location, ok := maas.EndpointConfig(m.ActualModelName)
	if !ok {
		return "", errors.New("unknown Vertex MaaS model endpoint")
	}
	host, location = a.applyCustomHostAndRegion(host, location, m)
	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/endpoints/openapi/chat/completions", host, m.Config.VertexAIProjectID, location), nil
}
