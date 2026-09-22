import { strict as assert } from 'node:assert';
import { execFileSync } from 'node:child_process';
import { describe, it } from 'vitest';
import { buildModelApiExamples, normalizeApiBaseUrl, quoteShellArgument, type ModelApiExample, type ModelApiMetadata } from './model-api-examples';
import { MODEL_API_REVIEWED_ON, MODEL_API_SOURCES, resolveModelApiProfile } from './model-api-profiles';

const BASE_URL = 'https://gateway.example';
// Each row asserts a nonempty, reviewed gateway contract. Uncertain aliases and
// native media protocols have separate negative cases below, not fake success.
const cases: Array<[string, ModelApiMetadata, string, string]> = [
  ['gpt-4o', {}, 'chat', '/v1/chat/completions'],
  ['claude-sonnet-4-5', {}, 'chat', '/v1/chat/completions'],
  ['gemini-2.5-pro', { input_modalities: ['text', 'image', 'audio', 'video'], output_modalities: ['text'] }, 'chat', '/v1/chat/completions'],
  ['openai/gpt-5-codex', {}, 'responses', '/v1/responses'],
  ['gpt-5-pro', {}, 'responses', '/v1/responses'],
  ['text-embedding-3-small', {}, 'embeddings', '/v1/embeddings'],
  ['jina-embeddings-v3', {}, 'embeddings', '/v1/embeddings'],
  ['jina-clip-v2', {}, 'embeddings', '/v1/embeddings'],
  ['text2vec-base-chinese', {}, 'embeddings', '/v1/embeddings'],
  ['Qwen/Qwen3-Embedding-8B', { image_pricing: {}, video_pricing: {} }, 'embeddings', '/v1/embeddings'],
  ['multimodal-encoder', { supported_features: ['embeddings'], embedding_pricing: {}, image_pricing: {}, video_pricing: {} }, 'embeddings', '/v1/embeddings'],
  ['bge-reranker-v2-m3', {}, 'rerank', '/v1/rerank'],
  ['rerank-v4.0-pro', {}, 'rerank', '/v1/rerank'],
  ['jina-reranker-v3.5', {}, 'rerank', '/v1/rerank'],
  ['dall-e-3', {}, 'image', '/v1/images/generations'],
  ['gpt-image-1.5', {}, 'image', '/v1/images/generations'],
  ['grok-imagine-image', {}, 'image', '/v1/images/generations'],
  ['cogview-4', {}, 'image', '/v1/images/generations'],
  ['sora-2', {}, 'video', '/v1/videos'],
  ['whisper-1', {}, 'transcription', '/v1/audio/transcriptions'],
  ['whisper-large-v3-turbo', {}, 'transcription', '/v1/audio/transcriptions'],
  ['gpt-4o-mini-transcribe', {}, 'transcription', '/v1/audio/transcriptions'],
  ['gpt-4o-transcribe-diarize', {}, 'diarization', '/v1/audio/transcriptions'],
  ['glm-asr-2512', {}, 'transcription', '/v1/audio/transcriptions'],
  ['tts-1', {}, 'speech', '/v1/audio/speech'],
  ['gpt-4o-mini-tts', {}, 'speech', '/v1/audio/speech'],
  ['glm-tts', { output_modalities: ['audio'] }, 'speech', '/v1/audio/speech'],
  ['glm-realtime-flash', { output_modalities: ['audio'] }, 'realtime', '/v1/realtime'],
  ['gemini-3.8-live', { output_modalities: ['audio'] }, 'gemini_live', '/v1/realtime'],
  ['omni-moderation-latest', {}, 'moderation', '/v1/moderations'],
  ['glm-ocr', {}, 'ocr', '/api/paas/v4/layout_parsing'],
  ['jina-ocr-v1', {}, 'jina_ocr', '/v1/chat/completions'],
  ['glm-tts-clone', { output_modalities: ['audio'] }, 'clone', '/v1/voice/clones'],
  ['gpt-3.5-turbo-instruct', {}, 'completions', '/v1/completions'],
];

/** captureCurlArguments executes generated shell syntax with a fake curl that never makes network requests. */
function captureCurlArguments(command: string): string[] {
  const output = execFileSync('sh', ['-c', `curl() { printf '%s\\0' "$@"; }\n${command}`], { encoding: 'utf8' });
  return output.split('\0').slice(0, -1);
}

/** requestBody decodes the JSON argument actually delivered to the fake curl command. */
function requestBody(example: ModelApiExample): Record<string, unknown> {
  const args = captureCurlArguments(example.request);
  assert.ok(args.includes('--data-raw'));
  return JSON.parse(args[args.indexOf('--data-raw') + 1]);
}

const shellTest = process.platform === 'win32' ? it.skip : it;

describe('model API examples', () => {
  for (const [model, metadata, id, path] of cases) {
    it(`selects ${id} for ${model}`, () => {
      const examples = buildModelApiExamples(model, metadata, BASE_URL);
      assert.ok(examples.length > 0);
      assert.equal(examples[0].id, id);
      assert.equal(new URL(examples[0].endpoint).pathname, path);
      for (const example of examples) {
        assert.equal(new URL(example.endpoint).origin, BASE_URL);
        assert.ok(example.request.includes('Authorization: Bearer YOUR_API_KEY'));
        assert.ok(example.response.length > 0);
        assert.ok(Object.values(MODEL_API_SOURCES).includes(example.source as typeof MODEL_API_SOURCES[keyof typeof MODEL_API_SOURCES]));
        assert.equal(new URL(example.source!).protocol, 'https:');
        assert.equal(example.reviewedOn, MODEL_API_REVIEWED_ON);
        if (example.responseFormat === 'json') assert.doesNotThrow(() => JSON.parse(example.response));
      }
    });
  }

  shellTest('all examples send the selected model and documented endpoint without calling a real API', () => {
    for (const [model, metadata] of cases) {
      const examples = buildModelApiExamples(model, metadata, BASE_URL);
      assert.ok(examples.length > 0, model);
      for (const example of examples) {
        const args = captureCurlArguments(example.request);
        assert.equal(args[args.indexOf('--request') + 1], example.method);
        assert.equal(args[args.indexOf('--request') + 2], example.endpoint);
        if (example.method === 'GET') {
          assert.equal(new URL(example.endpoint).searchParams.get('model'), model);
        } else if (args.includes('--data-raw')) {
          assert.equal(requestBody(example).model, model);
        } else {
          assert.ok(args.includes(`model=${model}`));
          assert.ok(args.includes('--form'));
          assert.ok(!args.some((arg) => arg.toLowerCase().startsWith('content-type:')));
        }
      }
    }
  });

  shellTest('shell metacharacters in explicitly typed aliases remain literal JSON data', () => {
    const model = `tenant/it's "special" \\ $(printf INJECTED); \`printf INJECTED\`\nmodel`;
    const examples = buildModelApiExamples(model, { supported_features: ['systemone'] }, BASE_URL);
    assert.equal(examples.length, 1, 'security tests must not pass vacuously');
    assert.equal(requestBody(examples[0]).model, model);
  });

  shellTest('multipart names and hostile gateway prefixes remain single literal arguments', () => {
    const model = "tenant'$(printf INJECTED)/whisper-1";
    const [example] = buildModelApiExamples(model, {}, "https://gateway.example/it's-a-prefix");
    const args = captureCurlArguments(example.request);
    assert.equal(args[2], example.endpoint);
    assert.equal(args[args.indexOf('--form-string') + 1], `model=${model}`);
    assert.equal(args[args.indexOf('--form') + 1], 'file=@audio.wav');
  });

  shellTest('text formats have their own request and response contracts', () => {
    const [chat, responses, messages] = buildModelApiExamples('gpt-4o', {}, BASE_URL);
    assert.ok(Array.isArray(requestBody(chat).messages));
    assert.equal(requestBody(responses).input, 'Say hello.');
    assert.equal(requestBody(messages).max_tokens, 1024);
    assert.ok(captureCurlArguments(messages.request).includes('anthropic-version: 2023-06-01'));
    assert.equal(JSON.parse(chat.response).object, 'chat.completion');
    assert.equal(JSON.parse(responses.response).object, 'response');
    assert.equal(JSON.parse(messages.response).type, 'message');
  });

  shellTest('GPT Image omits response_format while DALL-E explicitly requests base64', () => {
    assert.equal(requestBody(buildModelApiExamples('gpt-image-1', {}, BASE_URL)[0]).response_format, undefined);
    assert.equal(requestBody(buildModelApiExamples('dall-e-3', {}, BASE_URL)[0]).response_format, 'b64_json');
    const edits = buildModelApiExamples('dall-e-2', {}, BASE_URL).find(({ id }) => id === 'image_edit');
    assert.ok(edits);
    assert.ok(captureCurlArguments(edits.request).includes('response_format=b64_json'));
  });

  shellTest('image requests preserve valid catalog sizes without copying foreign provider resolution labels', () => {
    assert.equal(requestBody(buildModelApiExamples('gpt-image-1', { image_pricing: { default_size: '1024x1536' } }, BASE_URL)[0]).size, '1024x1536');
    assert.equal(requestBody(buildModelApiExamples('dall-e-3', { image_pricing: { default_size: '1k' } }, BASE_URL)[0]).size, undefined);
  });

  shellTest('speech requests save a binary response rather than pretending it is JSON', () => {
    const [example] = buildModelApiExamples('tts-1', {}, BASE_URL);
    const args = captureCurlArguments(example.request);
    assert.equal(requestBody(example).response_format, 'wav');
    assert.equal(args[args.indexOf('--output') + 1], 'speech.wav');
    assert.equal(example.responseFormat, 'http');
    assert.ok(example.response.includes('Content-Type: audio/wav'));
  });

  it('only the reviewed Whisper-1 profile includes audio translation', () => {
    assert.deepEqual(buildModelApiExamples('whisper-1', {}, BASE_URL).map((item) => item.id), ['transcription', 'translation']);
    for (const model of ['gpt-4o-transcribe', 'whisper-large-v3-turbo', 'glm-asr-2512']) {
      assert.deepEqual(buildModelApiExamples(model, {}, BASE_URL).map((item) => item.id), ['transcription']);
    }
  });

  it('audio and per-call pricing do not turn a conversational model into TTS or reranking', () => {
    const data = { input_modalities: ['text', 'audio'], output_modalities: ['text', 'audio'], audio_pricing: {}, per_call_pricing: {} };
    assert.equal(buildModelApiExamples('gpt-4o-audio-preview', data, BASE_URL)[0].id, 'chat');
  });

  it('video examples show queued jobs, and realtime examples show only the HTTP upgrade', () => {
    const [video] = buildModelApiExamples('sora-2', {}, BASE_URL);
    assert.equal(JSON.parse(video.response).status, 'queued');
    assert.equal(video.note, 'video');
    const [realtime] = buildModelApiExamples('gpt-realtime', {}, BASE_URL);
    assert.equal(realtime.method, 'GET');
    assert.equal(realtime.note, 'realtime');
    assert.ok(realtime.response.startsWith('HTTP/1.1 101'));
  });

  it('preserves gateway path prefixes and strips a trailing v1 only once', () => {
    assert.equal(normalizeApiBaseUrl('https://gateway.example/one-api/v1/?debug=1#section'), 'https://gateway.example/one-api');
    assert.equal(buildModelApiExamples('gpt-4o', {}, 'https://gateway.example/one-api/v1/')[0].endpoint, 'https://gateway.example/one-api/v1/chat/completions');
    assert.equal(buildModelApiExamples('glm-ocr', {}, 'https://gateway.example/v1')[0].endpoint, 'https://gateway.example/api/paas/v4/layout_parsing');
  });

  it('does not expose credentials or accept non-HTTP base URLs', () => {
    for (const base of ['', 'not a URL', 'javascript:alert(1)', 'https://user:secret@gateway.example']) {
      const [example] = buildModelApiExamples('gpt-4o', {}, base);
      assert.equal(new URL(example.endpoint).origin, 'https://your-one-api.example');
      assert.ok(!example.request.includes('secret'));
    }
    assert.equal(normalizeApiBaseUrl('http://localhost:3000/'), 'http://localhost:3000');
    assert.equal(quoteShellArgument(''), "''");
  });

  it('treats unknown and conflicting task metadata as unverified, not a chat fallback', () => {
    for (const model of ['my-private-alias', 'custom-video', 'gemini-image-model', 'Qwen/Qwen-Image-Edit', 'grok-unverified-video', 'mistral-ocr-latest', 'deepl', 'gpt-oss-safeguard', 'qwen3-omni', 'o3-deep-research']) {
      assert.deepEqual(buildModelApiExamples(model, {}, BASE_URL), [], model);
    }
    assert.deepEqual(buildModelApiExamples('alias', { supported_features: ['systemone', 'embeddings'] }, BASE_URL), []);
    assert.deepEqual(buildModelApiExamples('alias', { embedding_pricing: {} }, BASE_URL), []);
    assert.equal(resolveModelApiProfile('veo-3.1-generate-preview', {}).reason, 'native');
  });

  it('matching is case-insensitive while requests preserve the original routing key and model namespace', () => {
    for (const model of ['JinaAI/JINA-CLIP-V2', 'OpenAI/GPT-5-PRO', 'Google/GEMINI-3.8-LIVE']) {
      const examples = buildModelApiExamples(model, {}, BASE_URL);
      assert.ok(examples.length > 0);
      assert.ok(examples[0].request.includes(model) || examples[0].endpoint.includes(encodeURIComponent(model)));
    }
    assert.equal(buildModelApiExamples('alias', { supported_features: [' EMBEDDINGS '] }, BASE_URL)[0].id, 'embeddings');
  });
});
