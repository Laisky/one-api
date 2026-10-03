import { strict as assert } from 'node:assert';
import { describe, it } from 'vitest';
import { buildModelApiExamples } from './model-api-examples';
import { MODEL_API_SOURCES, resolveModelApiProfile } from './model-api-profiles';

/** examples returns generated templates using the supplied model at an inert gateway URL. */
function examples(model: string) {
  return buildModelApiExamples(model, {}, 'https://gateway.example');
}

describe('model profile boundaries and references', () => {
  for (const model of ['mistral-embed', 'codestral-embed', 'mistralai/codestral-embed-2505']) {
    it(`uses an embedding contract rather than chat for ${model}`, () => {
      assert.equal(examples(model)[0].id, 'embeddings');
      assert.equal(examples(model)[0].source, MODEL_API_SOURCES.mistral_embed);
    });
  }
  for (const model of ['mistral-moderation-latest', 'llama-guard-4', 'Qwen3-8B-Base', 'gpt-oss-safeguard-20b', 'gpt-live', 'gpt-5-deep-research']) {
    it(`does not silently route specialized ${model} into plain chat`, () => {
      assert.deepEqual(examples(model), []);
    });
  }
  for (const model of ['o1', 'o3', 'o4-mini', 'deepseek-v3', 'Qwen/Qwen3-32B', 'meta-llama/Llama-4-Maverick']) {
    it(`retains ordinary gateway chat formats for ${model}`, () => {
      assert.deepEqual(examples(model).map(({ id }) => id), ['chat', 'responses', 'messages']);
    });
  }
  it('GLM ASR omits unverified optional response_format while other transcription examples request JSON', () => {
    assert.doesNotMatch(examples('glm-asr-2512')[0].request, /response_format=/);
    assert.match(examples('whisper-large-v3-turbo')[0].request, /response_format=json/);
    assert.equal(examples('whisper-large-v3-turbo')[0].source, MODEL_API_SOURCES.groq_asr);
  });
  it('conflicting explicit task metadata does not guess a codec', () => {
    assert.equal(resolveModelApiProfile('private-model', { supported_features: ['embeddings', 'rerank'] }).kind, 'unverified');
  });
  it('all generated samples carry HTTPS protocol references and a review date', () => {
    for (const model of ['gpt-4o', 'gpt-5-pro', 'jev-latest', 'jina-clip-v2', 'jina-ocr-v1', 'glm-ocr', 'glm-tts-clone', 'gpt-realtime', 'gemini-3.8-live', 'gpt-4o-transcribe-diarize', 'whisper-1', 'glm-tts', 'sora-2', 'cogview-4', 'grok-imagine-image', 'gpt-image-1', 'dall-e-2', 'gpt-3.5-turbo-instruct', 'omni-moderation-latest', 'rerank-v4.0-pro']) {
      assert.ok(examples(model).length > 0, model);
      for (const example of examples(model)) {
        assert.equal(new URL(example.source!).protocol, 'https:');
        assert.equal(example.reviewedOn, '2026-09-18');
      }
    }
  });
});
