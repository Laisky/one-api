package deepinfra

// This file registers the embedding, speech, and image models from the
// DeepInfra catalog published on 2026-08-19 whose relay protocol and published
// pricing can be represented exactly by one-api. Iteration-priced and
// output-token-priced image models, video/world/music/OCR models, and the
// image-document reranker are intentionally excluded.
func init() {
	addCatalogModel("nvidia/llama-nemotron-embed-vl-1b-v2", catalogEmbeddingModel(0.01, 10000, []string{"text", "image"}, "Nemotron VL 1B multimodal embedding model."))
	addCatalogModel("sentence-transformers/clip-ViT-B-32", catalogEmbeddingModel(0.005, 77, []string{"text", "image"}, "CLIP ViT-B/32 text and image embedding model."))
	addCatalogModel("sentence-transformers/clip-ViT-B-32-multilingual-v1", catalogEmbeddingModel(0.005, 512, []string{"text", "image"}, "Multilingual CLIP ViT-B/32 embedding model."))
	addCatalogModel("shibing624/text2vec-base-chinese", catalogEmbeddingModel(0.005, 512, []string{"text"}, "Chinese Text2Vec base embedding model."))
	addCatalogModel("thenlper/gte-base", catalogEmbeddingModel(0.005, 512, []string{"text"}, "GTE base text embedding model."))
	addCatalogModel("thenlper/gte-large", catalogEmbeddingModel(0.01, 512, []string{"text"}, "GTE large text embedding model."))
	addCatalogModel("XiaomiMiMo/MiMo-V2.5-tts", characterPricedAudioModel(0, "MiMo V2.5 text-to-speech model."))
	addCatalogModel("XiaomiMiMo/MiMo-V2.5-tts-voiceclone", characterPricedAudioModel(0, "MiMo V2.5 voice-cloning speech model."))
	addCatalogModel("XiaomiMiMo/MiMo-V2.5-tts-voicedesign", characterPricedAudioModel(0, "MiMo V2.5 voice-design speech model."))
	addCatalogModel("Bria/Bria-3.2", flatImageModel(0.04, false, "Bria 3.2 commercial text-to-image model."))
	addCatalogModel("Bria/Bria-3.2-vector", flatImageModel(0.04, false, "Bria 3.2 Vector text-to-vector-image model."))
	addCatalogModel("Bria/blur_background", flatImageModel(0.04, true, "Bria licensed-data background blur model."))
	addCatalogModel("Bria/enhance", flatImageModel(0.04, true, "Bria licensed-data image enhancement model."))
	addCatalogModel("Bria/erase", flatImageModel(0.04, true, "Bria licensed-data object eraser."))
	addCatalogModel("Bria/erase_foreground", flatImageModel(0.04, true, "Bria licensed-data foreground eraser."))
	addCatalogModel("Bria/expand", flatImageModel(0.04, true, "Bria licensed-data image expansion model."))
	addCatalogModel("Bria/fibo", flatImageModel(0.04, false, "Bria FIBO structured text-to-image model."))
	addCatalogModel("Bria/fibo_edit", flatImageModel(0.04, true, "Bria FIBO image editing model."))
	addCatalogModel("Bria/gen_fill", flatImageModel(0.04, true, "Bria licensed-data generative fill model."))
	addCatalogModel("Bria/remove_background", flatImageModel(0.018, true, "Bria background removal model."))
	addCatalogModel("Bria/replace_background", flatImageModel(0.04, true, "Bria background replacement model."))
	addCatalogModel("ByteDance/Seedream-4", flatImageModel(0.04, true, "Seedream 4 multimodal image generation and editing model."))
	addCatalogModel("ByteDance/Seedream-4.5", flatImageModel(0.04, true, "Seedream 4.5 multimodal image generation and editing model."))
	addCatalogModel("ClarityAI/creative", flatImageModel(0.05, true, "ClarityAI creative image upscaler."))
	addCatalogModel("ClarityAI/crystal", flatImageModel(0.05, true, "ClarityAI portrait and product upscaler."))
	addCatalogModel("ClarityAI/flux", flatImageModel(0.2, true, "ClarityAI FLUX-powered image upscaler."))
	addCatalogModel("PrunaAI/p-image", flatImageModel(0.005, false, "Pruna P-Image real-time generation model."))
	addCatalogModel("PrunaAI/p-image-Edit", flatImageModel(0.01, true, "Pruna P-Image-Edit transformation model."))
	addCatalogModel("Qwen/Qwen-Image-Edit-Max", flatImageModel(0.075, true, "Qwen Image Edit Max model."))
	addCatalogModel("Qwen/Qwen-Image-Max", flatImageModel(0.075, false, "Qwen Image Max generation model."))
	addCatalogModel("Wan-AI/Wan2.6-Image-Edit", flatImageModel(0.03, true, "Wan 2.6 image generation and editing model."))
	addCatalogModel("Wan-AI/Wan2.6-T2I", flatImageModel(0.03, false, "Wan 2.6 text-to-image model."))
	addCatalogModel("Wan-AI/Wan2.7-Image-Edit", flatImageModel(0.03, true, "Wan 2.7 image generation and editing model."))
	addCatalogModel("black-forest-labs/FLUX-1.1-pro", flatImageModel(0.04, false, "FLUX 1.1 Pro text-to-image model."))
	addCatalogModel("black-forest-labs/FLUX-2-max", flatImageModel(0.07, true, "FLUX 2 Max image generation and editing model."))
	addCatalogModel("black-forest-labs/FLUX-2-pro", flatImageModel(0.015, true, "FLUX 2 Pro image generation and editing model."))
	addCatalogModel("deepseek-ai/Janus-Pro-1B", flatImageModel(0.0005, false, "Janus Pro 1B image generation model."))
	addCatalogModel("deepseek-ai/Janus-Pro-7B", flatImageModel(0.002, false, "Janus Pro 7B image generation model."))
}
