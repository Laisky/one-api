package router

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	// e2eDeepInfraKey is the credential of the DeepInfra fixture channel.
	e2eDeepInfraKey = "e2e-deepinfra-key"
	// e2eDeepInfraBase is DeepInfra's production base URL.
	e2eDeepInfraBase = "https://api.deepinfra.com"
	// e2eFluxMax and e2eFluxPro are DeepInfra's megapixel-priced FLUX.2 models.
	e2eFluxMax = "black-forest-labs/FLUX-2-max"
	e2eFluxPro = "black-forest-labs/FLUX-2-pro"
	// e2eFlux11Pro and e2eFluxKlein are flat and size-table priced controls.
	e2eFlux11Pro = "black-forest-labs/FLUX-1.1-pro"
	e2eFluxKlein = "black-forest-labs/FLUX-2-klein-4b"
)

// e2eDeepInfraImageEnv builds a gateway with one DeepInfra channel serving
// both FLUX.2 tiers and a fake upstream that returns one image per call.
// Parameters: t owns cleanup. Returns: the env.
func e2eDeepInfraImageEnv(t *testing.T) *e2eEnv {
	t.Helper()
	return newE2EEnv(t, func(e2eCall) e2eReply { return e2eImageReply("") },
		e2eChannel{ID: 939861, Type: channeltype.DeepInfra, Key: e2eDeepInfraKey, BaseURL: e2eDeepInfraBase,
			Models: e2eFluxMax + "," + e2eFluxPro + "," + e2eFlux11Pro + "," + e2eFluxKlein})
}

// e2ePNG encodes a blank PNG of the given size. Parameters: t owns
// assertions and width/height are the pixel dimensions. Returns: PNG bytes.
func e2ePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, image.NewGray(image.Rect(0, 0, width, height))))
	return out.Bytes()
}

// e2eImageEditForm renders a multipart Images edit request. Parameters: t
// owns assertions, fields are text fields, and files maps a form field to the
// uploaded file bytes. Returns: the body and its Content-Type.
func e2eImageEditForm(t *testing.T, fields map[string]string, files map[string][]byte) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		require.NoError(t, writer.WriteField(name, value))
	}
	for name, content := range files {
		part, err := writer.CreateFormFile(name, name+".png")
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

// postMultipart sends one authenticated multipart POST through the shipped
// router and waits for detached billing. Parameters: t owns assertions, path is
// the endpoint, and body/contentType form the request. Returns: the result.
func (e *e2eEnv) postMultipart(t *testing.T, path string, body []byte, contentType string) e2eResult {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-"+e.key)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	drainCriticalTasksForRouter(t)
	return e2eResult{Code: w.Code, Body: w.Body.String(), Header: w.Header(), RequestID: w.Header().Get(helper.RequestIdKey)}
}

// TestE2EDeepInfraFlux2MegapixelGenerations proves DeepInfra FLUX.2 [max] and
// [pro] generations are charged DeepInfra's published megapixel tariff ("The
// first megapixel is charged $X and then each subsequent MP will be charged
// $Y"), with BFL's 1,048,576-pixel megapixel rounded up per started MP, and
// that admission reserves exactly the settled charge. Quota is $1 = 500,000.
func TestE2EDeepInfraFlux2MegapixelGenerations(t *testing.T) {
	for _, tc := range []struct {
		name, model, body string
		charge            int64
	}{
		// 1024x1024 is exactly 1 MP: $0.07 / $0.03.
		{"max_default_size", e2eFluxMax, `{"model":"` + e2eFluxMax + `","prompt":"a fox"}`, 35_000},
		{"pro_default_size", e2eFluxPro, `{"model":"` + e2eFluxPro + `","prompt":"a fox"}`, 15_000},
		// 1440x1440 = 2,073,600 px = 1.98 MP, billed as 2 MP: $0.07 + $0.03.
		{"max_1440", e2eFluxMax, `{"model":"` + e2eFluxMax + `","prompt":"a fox","size":"1440x1440"}`, 50_000},
		// Two images at 2 MP each: 2 x $0.10.
		{"max_1440_n2", e2eFluxMax, `{"model":"` + e2eFluxMax + `","prompt":"a fox","size":"1440x1440","n":2}`, 100_000},
		// $0.03 + $0.015.
		{"pro_1440", e2eFluxPro, `{"model":"` + e2eFluxPro + `","prompt":"a fox","size":"1440x1440"}`, 22_500},
		// 1000x1048 = 1,048,000 px is below 1 MP, but DeepInfra sides are
		// multiples of 32, so it is billed as 1024x1056 = 2 MP.
		{"max_off_grid_rounds_up", e2eFluxMax, `{"model":"` + e2eFluxMax + `","prompt":"a fox","size":"1000x1048"}`, 50_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := e2eDeepInfraImageEnv(t)
			before := env.ledger(t)

			res := env.post(t, "/v1/images/generations", tc.body)

			e2eRequireImageWire(t, res)
			calls := env.upstream.callsFor(e2eDeepInfraKey)
			require.Len(t, calls, 1)
			require.Equal(t, "/v1/openai/images/generations", calls[0].Path)
			require.Equal(t, tc.charge, before.UserQuota-calls[0].UserQuota, "admission must reserve the megapixel charge")
			row := env.requireSettled(t, res, before, tc.charge, false)
			require.Contains(t, row.Content, "output_mp=")
		})
	}
}

// TestE2EDeepInfraFlux2MegapixelEdits proves DeepInfra FLUX.2 edits also pay
// "$Z for each megapixel on the input (i.e. reference images for editing)",
// measured from the uploaded image's decoded header and rounded up per image,
// and that an undecodable reference is billed at the conservative 16 MP
// fallback and flagged as an estimate.
func TestE2EDeepInfraFlux2MegapixelEdits(t *testing.T) {
	// 1500x1000 = 1,500,000 px = 1.43 MP, billed as 2 MP.
	reference := e2ePNG(t, 1500, 1000)
	for _, tc := range []struct {
		name, model string
		image       []byte
		charge      int64
		estimated   bool
	}{
		// $0.07 (1 MP output) + 2 x $0.03 (reference).
		{"max_reference", e2eFluxMax, reference, 65_000, false},
		// $0.03 + 2 x $0.015.
		{"pro_reference", e2eFluxPro, reference, 30_000, false},
		// $0.07 + 16 x $0.03 for a reference whose size cannot be decoded.
		{"max_undecodable_reference", e2eFluxMax, []byte("not an image header"), 275_000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := e2eDeepInfraImageEnv(t)
			before := env.ledger(t)
			body, contentType := e2eImageEditForm(t,
				map[string]string{"model": tc.model, "prompt": "add a hat", "size": "1024x1024"},
				map[string][]byte{"image": tc.image})

			res := env.postMultipart(t, "/v1/images/edits", body, contentType)

			e2eRequireImageWire(t, res)
			calls := env.upstream.callsFor(e2eDeepInfraKey)
			require.Len(t, calls, 1)
			require.Equal(t, "/v1/images/edits", calls[0].Path)
			require.Equal(t, body, calls[0].Raw, "the multipart upload is forwarded unchanged")
			require.Equal(t, tc.charge, before.UserQuota-calls[0].UserQuota, "admission must reserve the megapixel charge")
			row := env.requireSettled(t, res, before, tc.charge, tc.estimated)
			require.Contains(t, row.Content, "references=1")
		})
	}
}

// TestE2EDeepInfraFlux2MegapixelRefusals proves requests the megapixel tariff
// cannot measure are refused before any reservation or provider call: a size
// that is not WIDTHxHEIGHT, and an edit whose reference images are not uploaded
// as multipart files.
func TestE2EDeepInfraFlux2MegapixelRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, code string
	}{
		{"unmeasurable_size", "/v1/images/generations", `{"model":"` + e2eFluxMax + `","prompt":"a fox","size":"auto"}`, "size_not_supported"},
		{"json_edit", "/v1/images/edits", `{"model":"` + e2eFluxMax + `","prompt":"add a hat","image":"https://example.com/a.png"}`, "invalid_image_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := e2eDeepInfraImageEnv(t)
			before := env.ledger(t)

			res := env.post(t, tc.path, tc.body)

			require.Equal(t, http.StatusBadRequest, res.Code, "body=%s", res.Body)
			require.Contains(t, res.Body, tc.code)
			require.Zero(t, env.upstream.total(), "an unmeasurable request must never reach the provider")
			env.requireNothingCharged(t, res, before)
		})
	}
}

// TestE2EDeepInfraNonMegapixelImageTariffsUnchanged proves models that do not
// opt into megapixel pricing keep their existing charges: FLUX 1.1 [pro] stays
// a flat $0.04 at any size and FLUX.2 [klein] keeps its size table.
func TestE2EDeepInfraNonMegapixelImageTariffsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		charge     int64
	}{
		{"flat_large_size", `{"model":"` + e2eFlux11Pro + `","prompt":"a fox","size":"1440x1440"}`, 20_000},
		// $0.014 x 1.5 for 1024x1536.
		{"size_table", `{"model":"` + e2eFluxKlein + `","prompt":"a fox","size":"1024x1536"}`, 10_500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := e2eDeepInfraImageEnv(t)
			before := env.ledger(t)

			res := env.post(t, "/v1/images/generations", tc.body)

			e2eRequireImageWire(t, res)
			row := env.requireSettled(t, res, before, tc.charge, false)
			require.NotContains(t, row.Content, "output_mp=")
		})
	}
}
