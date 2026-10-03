// Command model-protocol-audit exports catalog capabilities without calling providers.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// main writes only model capability metadata to the supplied output file.
// It never reads channel credentials or calls a paid provider endpoint.
func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: model-protocol-audit output.json")
		os.Exit(2)
	}
	rows := []map[string]any{}
	for ch := 1; ch < channeltype.Dummy; ch++ {
		a := relay.GetAdaptor(channeltype.ToAPIType(ch))
		if a == nil {
			continue
		}
		a.Init(&meta.Meta{ChannelType: ch})
		configs := a.GetDefaultModelPricing()
		names := append([]string(nil), a.GetModelList()...)
		sort.Strings(names)
		for _, name := range names {
			cfg := configs[name]
			rows = append(rows, map[string]any{"model": name, "provider": a.GetChannelName(), "channel_type": ch,
				"supported_features": cfg.SupportedFeatures, "input_modalities": cfg.InputModalities,
				"output_modalities": cfg.OutputModalities, "image_pricing": cfg.Image,
				"video_pricing": cfg.Video, "audio_pricing": cfg.Audio,
				"embedding_pricing": cfg.Embedding, "per_call_pricing": cfg.PerCall})
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err == nil {
		err = os.WriteFile(os.Args[1], data, 0600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "export catalog:", err)
		os.Exit(1)
	}
}
