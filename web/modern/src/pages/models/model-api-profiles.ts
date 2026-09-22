import type { ModelApiMetadata } from './model-api-examples';

/** ModelApiKind identifies an audited wire contract, not a modality or a billing unit. */
export type ModelApiKind =
  | 'chat' | 'responses' | 'systemone' | 'embeddings' | 'rerank' | 'moderation'
  | 'ocr' | 'jina_ocr' | 'clone' | 'realtime' | 'gemini_live' | 'transcription'
  | 'diarization' | 'speech' | 'video' | 'grok_video' | 'image' | 'cogview' | 'grok_image'
  | 'completions' | 'unverified';

/** ModelApiProfile records a template's protocol and the primary reference used in its audit. */
export interface ModelApiProfile {
  kind: ModelApiKind;
  source: string;
  reason?: 'unknown' | 'native';
}

export const MODEL_API_REVIEWED_ON = '2026-09-18';
export const MODEL_API_SOURCES = {
  chat: 'https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create',
  responses: 'https://developers.openai.com/api/docs/guides/migrate-to-responses',
  messages: 'https://platform.claude.com/docs/en/api/typescript/messages/create',
  systemone: 'https://docs.typesafe.ai/api',
  embeddings: 'https://developers.openai.com/api/reference/resources/embeddings/methods/create',
  jina: 'https://api.jina.ai/docs',
  rerank: 'https://docs.cohere.com/v2/reference/rerank',
  moderation: 'https://developers.openai.com/api/reference/resources/moderations/methods/create',
  ocr: 'https://docs.z.ai/api-reference/tools/layout-parsing',
  jina_ocr: 'https://jina.ai/news/jina-ocr-v1-faster-document-parsing-on-low-budget-gpus',
  // The provider's clone reference was unavailable; this pins the reviewed gateway contract.
  clone: 'https://github.com/Laisky/one-api/blob/0932e2853c0c51d6eba3a686b975a1fbca8fcc65/relay/adaptor/zhipu/voice_clone.go',
  realtime: 'https://developers.openai.com/api/docs/guides/voice-websockets',
  gemini_live: 'https://ai.google.dev/gemini-api/docs/live-api',
  transcription: 'https://developers.openai.com/api/docs/guides/speech-to-text',
  groq_asr: 'https://console.groq.com/docs/speech-to-text',
  mistral_embed: 'https://docs.mistral.ai/api/endpoint/embeddings',
  glm_asr: 'https://docs.z.ai/api-reference/audio/audio-transcriptions',
  speech: 'https://developers.openai.com/api/reference/resources/audio/subresources/speech/methods/create',
  glm_tts: 'https://docs.bigmodel.cn/api-reference/模型-api/文本转语音',
  video: 'https://developers.openai.com/api/docs/guides/video-generation',
  image: 'https://developers.openai.com/api/reference/resources/images/methods/generate',
  image_edit: 'https://developers.openai.com/api/reference/resources/images/methods/edit',
  cogview: 'https://docs.bigmodel.cn/api-reference/模型-api/图像生成',
  grok_image: 'https://docs.x.ai/developers/model-capabilities/images/generation',
  grok_video: 'https://docs.x.ai/developers/model-capabilities/video/generation',
  gemini_tts: 'https://ai.google.dev/gemini-api/docs/speech-generation',
  gemini_image: 'https://ai.google.dev/gemini-api/docs/image-generation',
  veo: 'https://ai.google.dev/gemini-api/docs/video',
  mistral_ocr: 'https://docs.mistral.ai/studio/document-processing/basic_ocr',
  completions: 'https://developers.openai.com/api/reference/resources/completions/methods/create',
} as const;

/** resolveModelApiProfile selects a reviewed contract from the supplied ID and explicit task features, or returns guidance. */
export function resolveModelApiProfile(model: string, data: ModelApiMetadata): ModelApiProfile {
  const name = model.toLowerCase().split('/').pop() || model.toLowerCase();
  const features = (data.supported_features ?? []).map((value) => value.trim().toLowerCase());
  const taskFeatures = [...new Set(features.filter((value) => ['systemone', 'rerank', 'embeddings'].includes(value)))];
  // known constructs the result from an audited protocol and its reference.
  const known = (kind: ModelApiKind, source: string): ModelApiProfile => ({ kind, source });
  // uncertain retains guidance rather than manufacturing a runnable request.
  const uncertain = (source = '', reason: 'unknown' | 'native' = 'unknown'): ModelApiProfile => ({ kind: 'unverified', source, reason });
  if (taskFeatures.length > 1) return uncertain();
  if (/^jev(?:-|$)/.test(name) || taskFeatures[0] === 'systemone') return known('systemone', MODEL_API_SOURCES.systemone);

  // Specific non-conversational tasks win over broad family names such as GPT,
  // Gemini, Qwen and GLM. Pricing objects never establish a request encoding.
  if (name === 'jina-ocr-v1') return known('jina_ocr', MODEL_API_SOURCES.jina_ocr);
  if (name === 'glm-ocr') return known('ocr', MODEL_API_SOURCES.ocr);
  if (name === 'glm-tts-clone') return known('clone', MODEL_API_SOURCES.clone);
  if (/^gemini-.*(?:live|native-audio)/.test(name)) return known('gemini_live', MODEL_API_SOURCES.gemini_live);
  if (/^gemini-.*tts/.test(name)) return uncertain(MODEL_API_SOURCES.gemini_tts, 'native');
  if (/^gemini-.*image|^imagen(?:-|$)/.test(name)) return uncertain(MODEL_API_SOURCES.gemini_image, 'native');
  if (/^veo(?:-|$)/.test(name)) return uncertain(MODEL_API_SOURCES.veo, 'native');
  if (/^grok-imagine-video(?:-1\.5(?:-preview|-2026-05-30)?|-2026-01-20)?$/.test(name)) return known('grok_video', MODEL_API_SOURCES.grok_video);
  if (/^grok-.*video/.test(name)) return uncertain(MODEL_API_SOURCES.grok_video, 'native');
  if (/^mistral-ocr/.test(name)) return uncertain(MODEL_API_SOURCES.mistral_ocr, 'native');

  if (/rerank/.test(name) || taskFeatures[0] === 'rerank') return known('rerank', name.startsWith('jina-') ? MODEL_API_SOURCES.jina : MODEL_API_SOURCES.rerank);
  if (/^(?:text-embedding-|embedding-|gemini-embedding-|jina-embeddings-|jina-clip-|text2vec|bge-|e5-|nomic-embed-text|mxbai-embed-large|snowflake-arctic-embed)|^qwen.*embedding/.test(name) || taskFeatures[0] === 'embeddings') {
    return known('embeddings', name.startsWith('jina-') ? MODEL_API_SOURCES.jina : MODEL_API_SOURCES.embeddings);
  }
  if (/^(?:mistral|codestral)-embed(?:-|$)/.test(name)) return known('embeddings', MODEL_API_SOURCES.mistral_embed);
  if (/^(?:omni|text)-moderation(?:-|$)/.test(name)) return known('moderation', MODEL_API_SOURCES.moderation);
  if (/^(?:gpt.*|glm)-realtime(?:-|$)/.test(name) || /^gpt-4o.*-realtime/.test(name)) return known('realtime', MODEL_API_SOURCES.realtime);
  if (/^gpt-.*transcribe-diarize(?:-|$)/.test(name)) return known('diarization', MODEL_API_SOURCES.transcription);
  if (/^(?:whisper-1$|whisper-large-v3(?:-turbo)?$|gpt-.*transcrib|glm-asr-)/.test(name)) return known('transcription', name.startsWith('glm-') ? MODEL_API_SOURCES.glm_asr : name.startsWith('whisper-large') ? MODEL_API_SOURCES.groq_asr : MODEL_API_SOURCES.transcription);
  if (/^tts-1(?:-hd)?$|^gpt-4o-mini-tts(?:-|$)|^glm-tts$/.test(name)) return known('speech', name === 'glm-tts' ? MODEL_API_SOURCES.glm_tts : MODEL_API_SOURCES.speech);
  if (/^sora-2(?:-|$)/.test(name)) return known('video', MODEL_API_SOURCES.video);
  if (/^cogview(?:-|$)/.test(name)) return known('cogview', MODEL_API_SOURCES.cogview);
  if (/^grok-(?:2-image|imagine-image)(?:-|$)/.test(name)) return known('grok_image', MODEL_API_SOURCES.grok_image);
  if (/^dall-e-[23]$|^gpt-image-|^chatgpt-image-latest$/.test(name)) return known('image', MODEL_API_SOURCES.image);
  if (/^text-(?:davinci|curie|babbage|ada)|^(?:davinci|babbage)-002$|^gpt-3\.5-turbo-instruct/.test(name)) return known('completions', MODEL_API_SOURCES.completions);

  // A native media or specialized workflow must not inherit a chat sample just
  // because its publisher also serves chat models. Add an audited profile first.
  if (/embed|rerank|ocr|tts|asr|speech|voice|audio|video|image|flux|diffusion|sdxl|kling|vidu|cogvideo|omni|guard|moderation|safety|classif|reward|computer-use|deep-research|deepl|voxtral|(?:^|-)base(?:-|$)|pretrained|gpt-live/.test(name)) {
    // These OpenAI models also support a text-only Chat Completions request.
    if (/^gpt-(?:4o(?:-mini)?-audio|audio)(?:-|$)/.test(name)) return known('chat', MODEL_API_SOURCES.chat);
    return uncertain();
  }
  if ((data.output_modalities ?? []).some((value) => value.toLowerCase() !== 'text')) return uncertain();
  if (data.embedding_pricing) return uncertain();
  // Prefer the provider-supported entry point even if a deployment happens to
  // enable extra gateway conversions for Responses-only models.
  if (/^gpt-.*codex|^gpt-\d[\d.]*-pro(?:-|$)|^o[13]-pro(?:-|$)/.test(name)) return known('responses', MODEL_API_SOURCES.responses);
  if (/^(?:gpt-(?:[3456]|oss)|chatgpt-|claude-|gemini-|deepseek-|qwen[\d.-]|(?:meta-)?llama[\d.-]|mistral-|mixtral-|gemma[\d.-]|command-(?:r|a)|grok-|glm-|yi-|moonshot-|kimi-|doubao-|ernie-|hunyuan-|jamba-|olmo-|phi-|llama-|o[134](?:-|$))/.test(name)) {
    return known('chat', MODEL_API_SOURCES.chat);
  }
  return uncertain();
}
