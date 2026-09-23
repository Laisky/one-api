package siliconflow

import "github.com/Laisky/one-api/relay/adaptor"

// init registers corrected public IDs while retaining historical gateway names
// and their administrator price keys. Capabilities describe the tested gateway
// contract, not every workflow offered by the underlying foundation model.
func init() {
	for historical, canonical := range map[string]string{
		"black-forest-labs/FLUX.1.1-pro": "black-forest-labs/FLUX-1.1-pro",
		"Bytedance/Z-Image-Turbo":        "Tongyi-MAI/Z-Image-Turbo",
	} {
		ModelRatios[canonical] = ModelRatios[historical].Clone()
	}
	for name, cfg := range ModelRatios {
		if cfg.Image == nil {
			continue
		}
		cfg = cfg.Clone()
		cfg.SupportedFeatures = append(cfg.SupportedFeatures, "gateway_siliconflow_image")
		cfg.InputModalities = []string{"text"}
		cfg.Image.MaxImages = 1
		if name == "black-forest-labs/FLUX.2-pro" {
			cfg.Image.DefaultSize = "512x512"
		}
		if name == "Tongyi-MAI/Z-Image-Turbo" || name == "Bytedance/Z-Image-Turbo" {
			cfg.Description = "Alibaba Tongyi Z-Image-Turbo text-to-image on SiliconFlow. The historical Bytedance gateway name redirects to Tongyi-MAI/Z-Image-Turbo."
		}
		ModelRatios[name] = cfg
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
