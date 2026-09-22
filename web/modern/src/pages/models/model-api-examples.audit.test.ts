import { strict as assert } from 'node:assert';
import { execFileSync } from 'node:child_process';
import { describe, it } from 'vitest';
import { buildModelApiExamples, type ModelApiExample, type ModelApiMetadata } from './model-api-examples';

const BASE = 'https://gateway.example';
const shellTest = process.platform === 'win32' ? it.skip : it;

/** argumentsFor evaluates a generated command with a fake curl and returns arguments without making network calls. */
function argumentsFor(example: ModelApiExample): string[] {
  return execFileSync('sh', ['-c', `curl() { printf '%s\\0' "$@"; }\n${example.request}`], { encoding: 'utf8' }).split('\0').slice(0, -1);
}

/** bodyFor decodes the JSON payload passed to the fake curl for the supplied example. */
function bodyFor(example: ModelApiExample) {
  const args = argumentsFor(example);
  assert.ok(args.includes('--data-raw'));
  return JSON.parse(args[args.indexOf('--data-raw') + 1]);
}

describe('model demo protocol audit regressions', () => {
  shellTest('Sora includes the duration required by gateway video admission', () => {
    const [example] = buildModelApiExamples('sora-2', {}, BASE);
    assert.ok(example);
    assert.equal(example.endpoint, `${BASE}/v1/videos`);
    assert.equal(bodyFor(example).seconds, '4');
    assert.equal(bodyFor(example).size, '1280x720');
    assert.equal(JSON.parse(example.response).object, 'video');
  });

  for (const model of ['gemini-3.8-live', 'gemini-live-2.5-flash-preview', 'gemini-2.5-flash-native-audio-preview-12-2025']) {
    it(`uses native Live handshake, not speech synthesis, for ${model}`, () => {
      const [example] = buildModelApiExamples(model, { output_modalities: ['audio'] }, BASE);
      assert.ok(example);
      assert.equal(example.id, 'gemini_live');
      assert.equal(example.method, 'GET');
      assert.equal(new URL(example.endpoint).pathname, '/v1/realtime');
      assert.equal(new URL(example.endpoint).searchParams.get('model'), model);
      assert.doesNotMatch(example.request, /OpenAI-Beta|audio\/speech/);
      assert.equal(example.note, 'geminiLive');
    });
  }

  shellTest('diarization selects the speaker-bearing format and handles long recordings', () => {
    const [example] = buildModelApiExamples('gpt-4o-transcribe-diarize', {}, BASE);
    const args = argumentsFor(example);
    assert.ok(args.includes('response_format=diarized_json'));
    assert.ok(args.includes('chunking_strategy=auto'));
    assert.ok(!args.includes('response_format=json'));
    assert.ok(!args.includes('Content-Type: multipart/form-data'));
    assert.equal(JSON.parse(example.response).segments[0].speaker, 'A');
  });

  shellTest('GLM speech uses its documented voice rather than an OpenAI voice', () => {
    const [example] = buildModelApiExamples('glm-tts', { output_modalities: ['audio'] }, BASE);
    assert.equal(bodyFor(example).voice, 'tongtong');
    assert.equal(bodyFor(example).response_format, 'wav');
    assert.equal(example.responseFormat, 'http');
  });

  shellTest('Jina OCR receives an image and returns Markdown in a chat envelope', () => {
    const [example] = buildModelApiExamples('jina-ocr-v1', { input_modalities: ['image'], output_modalities: ['text'] }, BASE);
    assert.equal(example.endpoint, `${BASE}/v1/chat/completions`);
    const content = bodyFor(example).messages[0].content;
    assert.ok(Array.isArray(content));
    assert.ok(content.some((part: { type: string }) => part.type === 'image_url'));
    assert.match(JSON.parse(example.response).choices[0].message.content, /Example document/);
  });

  it('Jina CLIP is an embedding model even without price metadata', () => {
    for (const model of ['jina-clip-v1', 'jinaai/jina-clip-v2']) {
      assert.equal(buildModelApiExamples(model, {}, BASE)[0].id, 'embeddings');
    }
  });

  shellTest('CogView displays the URL actually returned by its native image API', () => {
    const [example] = buildModelApiExamples('cogview-4', { image_pricing: {} }, BASE);
    assert.ok(JSON.parse(example.response).data[0].url);
    assert.equal(JSON.parse(example.response).data[0].b64_json, undefined);
    assert.equal(bodyFor(example).response_format, undefined);
  });

  it('Grok image models are recognized without optional pricing metadata', () => {
    assert.equal(buildModelApiExamples('grok-imagine-image', {}, BASE)[0].id, 'image');
  });

  it('GPT Image offers an actual multipart edit rather than requiring a fictional edit model ID', () => {
    const examples = buildModelApiExamples('gpt-image-1', {}, BASE);
    assert.deepEqual(examples.map(({ id }) => id), ['image', 'image_edit']);
    assert.match(examples[1].request, /image=@image.png/);
  });

  for (const model of ['veo-3.1-generate-preview', 'gemini-2.5-flash-preview-tts', 'gemini-2.5-flash-image', 'mistral-ocr-latest', 'deepl', 'grok-unverified-video', 'Qwen/Qwen-Image-Edit']) {
    it(`does not invent a gateway codec for ${model}`, () => {
      assert.deepEqual(buildModelApiExamples(model, {}, BASE), []);
    });
  }

  for (const data of [
    { input_modalities: ['text'], output_modalities: ['text'] },
    { output_modalities: ['audio'] },
    { video_pricing: {} },
    { image_pricing: {} },
  ]) {
    it(`does not turn uncertain metadata into a runnable demo: ${JSON.stringify(data)}`, () => {
      assert.deepEqual(buildModelApiExamples('unverified-private-alias', data, BASE), []);
    });
  }

  it('input image/video costs do not select generation for a known chat model', () => {
    assert.equal(buildModelApiExamples('gpt-4o', {
      input_modalities: ['text', 'image', 'video'], output_modalities: ['text'], image_pricing: {}, video_pricing: {},
    }, BASE)[0].id, 'chat');
  });

  it('Responses-only model examples do not advertise native chat compatibility', () => {
    assert.deepEqual(buildModelApiExamples('gpt-5-pro', {}, BASE).map(({ id }) => id), ['responses']);
    assert.deepEqual(buildModelApiExamples('openai/gpt-5-codex', {}, BASE).map(({ id }) => id), ['responses']);
  });

  it('normal chat, embedding and Jev contracts remain positive controls', () => {
    for (const [model, kind] of [['gpt-4o', 'chat'], ['text-embedding-3-small', 'embeddings'], ['jev-latest', 'systemone']]) {
      const examples = buildModelApiExamples(model, {}, BASE);
      assert.ok(examples.length > 0);
      assert.equal(examples[0].id, kind);
    }
  });
});
