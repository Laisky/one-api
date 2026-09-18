import { strict as assert } from 'node:assert';
import { execFileSync } from 'node:child_process';
import { describe, it } from 'vitest';
import { buildModelApiExamples, normalizeApiBaseUrl, quoteShellArgument, type ModelApiExample, type ModelApiMetadata } from './model-api-examples';

const BASE_URL = 'https://gateway.example';
const cases: Array<[string, ModelApiMetadata, string, string]> = [
  ['gpt-4o', {}, 'chat', '/v1/chat/completions'],
  ['claude-sonnet-4-5', {}, 'chat', '/v1/chat/completions'],
  ['gemini-2.5-pro', { input_modalities: ['text', 'image', 'audio', 'video'], output_modalities: ['text'] }, 'chat', '/v1/chat/completions'],
  ['my-private-alias', {}, 'chat', '/v1/chat/completions'],
  ['openai/gpt-5-codex', {}, 'responses', '/v1/responses'],
  ['text-embedding-3-small', {}, 'embeddings', '/v1/embeddings'],
  ['jina-embeddings-v3', {}, 'embeddings', '/v1/embeddings'],
  ['text2vec-base-chinese', {}, 'embeddings', '/v1/embeddings'],
  ['multimodal-encoder', { embedding_pricing: {}, image_pricing: {}, video_pricing: {} }, 'embeddings', '/v1/embeddings'],
  ['bge-reranker-v2-m3', {}, 'rerank', '/v1/rerank'],
  ['rerank-v4.0-pro', {}, 'rerank', '/v1/rerank'],
  ['dall-e-3', {}, 'image', '/v1/images/generations'],
  ['gpt-image-1.5', {}, 'image', '/v1/images/generations'],
  ['gemini-image-model', { output_modalities: ['text', 'image'] }, 'image', '/v1/images/generations'],
  ['Qwen/Qwen-Image-Edit', { output_modalities: ['image'] }, 'image_edit', '/v1/images/edits'],
  ['sora-2', {}, 'video', '/v1/videos'],
  ['veo-3.1-generate-preview', {}, 'video', '/v1/videos'],
  ['custom-video', { video_pricing: {} }, 'video', '/v1/videos'],
  ['whisper-1', {}, 'transcription', '/v1/audio/transcriptions'],
  ['gpt-4o-mini-transcribe', {}, 'transcription', '/v1/audio/transcriptions'],
  ['glm-asr-2512', {}, 'transcription', '/v1/audio/transcriptions'],
  ['tts-1', {}, 'speech', '/v1/audio/speech'],
  ['gpt-4o-mini-tts', {}, 'speech', '/v1/audio/speech'],
  ['glm-tts', { output_modalities: ['audio'] }, 'speech', '/v1/audio/speech'],
  ['glm-realtime-flash', { output_modalities: ['audio'] }, 'realtime', '/v1/realtime'],
  ['omni-moderation-latest', {}, 'moderation', '/v1/moderations'],
  ['glm-ocr', {}, 'ocr', '/api/paas/v4/layout_parsing'],
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
        if (example.responseFormat === 'json') assert.doesNotThrow(() => JSON.parse(example.response));
      }
    });
  }

  shellTest('all examples send the selected model and documented endpoint without calling a real API', () => {
    for (const [model, metadata] of cases) {
      for (const example of buildModelApiExamples(model, metadata, BASE_URL)) {
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

  shellTest('shell metacharacters in model names remain literal JSON data', () => {
    const model = `tenant/it's "special" \\ $(printf INJECTED); \`printf INJECTED\`\nmodel`;
    for (const example of buildModelApiExamples(model, {}, BASE_URL)) {
      assert.equal(requestBody(example).model, model);
    }
  });

  shellTest('multipart model names use form-string and cannot become file reads or shell commands', () => {
    const model = `@whisper-1'$(printf INJECTED)`;
    const [example] = buildModelApiExamples(model, {}, BASE_URL);
    const args = captureCurlArguments(example.request);
    assert.equal(args[args.indexOf('--form-string') + 1], `model=${model}`);
    assert.equal(args[args.indexOf('--form') + 1], 'file=@audio.wav');
  });

  shellTest('text formats have their own request and response contracts', () => {
    const [chat, responses, messages] = buildModelApiExamples('test-model', {}, BASE_URL);
    assert.ok(Array.isArray(requestBody(chat).messages));
    assert.equal(requestBody(responses).input, 'Say hello.');
    assert.equal(requestBody(messages).max_tokens, 1024);
    assert.ok(captureCurlArguments(messages.request).includes('anthropic-version: 2023-06-01'));
    assert.equal(JSON.parse(chat.response).object, 'chat.completion');
    assert.equal(JSON.parse(responses.response).object, 'response');
    assert.equal(JSON.parse(messages.response).type, 'message');
  });

  shellTest('GPT Image omits response_format while legacy image models explicitly request base64', () => {
    assert.equal(requestBody(buildModelApiExamples('gpt-image-1', {}, BASE_URL)[0]).response_format, undefined);
    assert.equal(requestBody(buildModelApiExamples('dall-e-3', {}, BASE_URL)[0]).response_format, 'b64_json');
  });

  shellTest('image requests preserve the catalog default size', () => {
    const [example] = buildModelApiExamples('custom-image', { image_pricing: { default_size: '1024x1536' } }, BASE_URL);
    assert.equal(requestBody(example).size, '1024x1536');
  });

  shellTest('speech requests save a binary response rather than pretending it is JSON', () => {
    const [example] = buildModelApiExamples('tts-1', {}, BASE_URL);
    const args = captureCurlArguments(example.request);
    assert.equal(requestBody(example).response_format, 'wav');
    assert.equal(args[args.indexOf('--output') + 1], 'speech.wav');
    assert.equal(example.responseFormat, 'http');
    assert.ok(example.response.includes('Content-Type: audio/wav'));
  });

  it('only Whisper examples include audio translation', () => {
    assert.deepEqual(buildModelApiExamples('whisper-1', {}, BASE_URL).map((item) => item.id), ['transcription', 'translation']);
    assert.deepEqual(buildModelApiExamples('gpt-4o-transcribe', {}, BASE_URL).map((item) => item.id), ['transcription']);
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
  });

  shellTest('quotes empty strings, apostrophes, and hostile gateway paths as single arguments', () => {
    assert.equal(quoteShellArgument(''), "''");
    const [example] = buildModelApiExamples('gpt-4o', {}, "https://gateway.example/it's-a-prefix");
    assert.equal(captureCurlArguments(example.request)[2], example.endpoint);
  });
});
