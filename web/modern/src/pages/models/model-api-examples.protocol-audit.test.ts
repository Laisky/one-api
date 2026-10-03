import { describe, expect, it } from 'vitest';
import { buildModelApiExamples, type ModelApiExample } from './model-api-examples';
import { resolveModelApiProfile } from './model-api-profiles';

/** payload reads the JSON emitted by these simple, quote-free example fixtures. */
function payload(example: ModelApiExample): Record<string, unknown> {
  const match = example.request.match(/--data-raw '([\s\S]*?)'(?:\s*\\\n|$)/);
  expect(match).not.toBeNull();
  return JSON.parse(match![1]) as Record<string, unknown>;
}

const base = 'https://gateway.example/prefix/v1';

describe('audited standardized model contracts', () => {
  it.each(['embed-v4.0', 'embed-english-v3.0', 'embed-english-light-v3.0', 'embed-multilingual-v3.0', 'embed-multilingual-light-v3.0'])('provides text embedding requests for %s', (model) => {
    const examples = buildModelApiExamples(model, {}, base);
    expect(examples).toHaveLength(1);
    expect(examples[0].endpoint).toBe('https://gateway.example/prefix/v1/embeddings');
    expect(payload(examples[0])).toEqual({ model, input: ['Hello, world!'] });
    expect(examples[0].source).toContain('docs.cohere.com');
    expect(JSON.parse(examples[0].response).data[0].index).toBe(0);
  });

  it.each([
    ['canopylabs/orpheus-v1-english', 'voice', 'troy'],
    ['canopylabs/orpheus-arabic-saudi', 'voice', 'fahad'],
    ['FunAudioLLM/CosyVoice2-0.5B', 'voice', 'FunAudioLLM/CosyVoice2-0.5B:alex'],
    ['voxtral-tts-2603', 'voice_id', 'YOUR_VOICE_ID'],
    ['voxtral-mini-tts-2603', 'voice_id', 'YOUR_VOICE_ID'],
    ['voxtral-mini-tts-latest', 'voice_id', 'YOUR_VOICE_ID'],
  ])('uses valid speech format and voice fields for %s', (model, field, voice) => {
    const [example] = buildModelApiExamples(model, {}, base);
    expect(example.endpoint).toBe('https://gateway.example/prefix/v1/audio/speech');
    const body = payload(example);
    expect(body[field]).toBe(voice);
    expect(body.response_format).toBe('wav');
    expect(body.model).toBe(model);
    expect(example.request).toContain('--output speech.wav');
    expect(example.note).toBe('meteredAudio');
    expect(example.responseFormat).toBe('http');
    expect(example.reviewedOn).toBe('2026-09-22');
  });

  it.each(['voxtral-mini-2602', 'voxtral-mini-transcribe-2602', 'whisper-large-v3-turbo'])('uses a single metered upload, without claiming translation, for %s', (model) => {
    const examples = buildModelApiExamples(model, {}, base);
    expect(examples).toHaveLength(1);
    expect(examples[0].endpoint.endsWith('/v1/audio/transcriptions')).toBe(true);
    expect(examples[0].request).toContain('file=@audio.wav');
    expect(examples[0].request).not.toContain('Content-Type:');
  });

  it.each(['cogvideox-2', 'cogvideox-3', 'cogvideox-flash', 'viduq1-text', 'viduq1-image', 'viduq1-start-end', 'vidu2-image', 'vidu2-start-end', 'vidu2-reference'])('creates and polls the native job for %s', (model) => {
    const [create, poll] = buildModelApiExamples(model, {}, base);
    expect(create.endpoint.endsWith('/v1/videos/generations')).toBe(true);
    expect(payload(create).model).toBe(model);
    expect(JSON.parse(create.response).task_status).toBe('PROCESSING');
    expect(JSON.parse(create.response).id).toBe('YOUR_TASK_ID');
    expect(poll.endpoint.endsWith('/v1/videos/YOUR_TASK_ID')).toBe(true);
    expect(poll.method).toBe('GET');
    expect(poll.request).not.toContain('--data');
    expect(poll.request).not.toContain('/content');
    expect(JSON.parse(poll.response).video_result).toHaveLength(1);
    if (model.endsWith('-start-end')) expect(payload(create).image_url).toHaveLength(2);
    else if (model.endsWith('-image')) expect(typeof payload(create).image_url).toBe('string');
    else if (model.endsWith('-reference')) expect(payload(create).image_url).toHaveLength(1);
    else expect(payload(create)).not.toHaveProperty('image_url');
  });

  it.each(['black-forest-labs/FLUX.1-schnell', 'black-forest-labs/FLUX.1.1-pro', 'black-forest-labs/FLUX-1.1-pro', 'black-forest-labs/FLUX.2-pro', 'black-forest-labs/FLUX.2-flex', 'Bytedance/Z-Image-Turbo', 'Tongyi-MAI/Z-Image-Turbo'])('requires the provider contract for %s', (model) => {
    expect(buildModelApiExamples(model, { image_pricing: { default_size: '1024x1024' } }, base)).toEqual([]);
    const [example] = buildModelApiExamples(model, { supported_features: ['gateway_siliconflow_image'] }, base);
    expect(example.endpoint.endsWith('/v1/images/generations')).toBe(true);
    expect(payload(example)).toMatchObject({ model, n: 1, response_format: 'url' });
    expect(payload(example).size).toBe(model.endsWith('FLUX.2-pro') ? '512x512' : '1024x1024');
    expect(JSON.parse(example.response).data).toHaveLength(1);
    expect(example.note).toBe('siliconflowImage');
  });

  it.each(['voxtral-made-up-tts', 'cogvideox-unknown', 'vidu3-image', 'canopylabs/orpheus-new'])('does not promote an unaudited model solely by name: %s', (model) => {
    expect(resolveModelApiProfile(model, {}).kind).toBe('unverified');
  });
});
