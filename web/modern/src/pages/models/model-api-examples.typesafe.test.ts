import { strict as assert } from 'node:assert';
import { execFileSync } from 'node:child_process';
import { describe, it } from 'vitest';
import { buildModelApiExamples, type ModelApiExample, type ModelApiMetadata } from './model-api-examples';

const BASE = 'https://gateway.example';
const TEXT_METADATA: ModelApiMetadata = { input_modalities: ['text'], output_modalities: ['text'] };

/** captureCurl executes a supplied example with a network-free shell function and returns its literal arguments. */
function captureCurl(example: ModelApiExample): string[] {
  return execFileSync('sh', ['-c', `curl() { printf '%s\\0' "$@"; }\n${example.request}`], { encoding: 'utf8' })
    .split('\0').slice(0, -1);
}

const shellTest = process.platform === 'win32' ? it.skip : it;

describe('TypeSafe System One model examples', () => {
  // The live catalog marks Jev as text-in/text-out, which is not evidence of chat support.
  for (const model of ['jev-latest', 'jev-preview', 'jev-1.13.0', 'typesafe/jev-latest', 'typesafe/jev-1.13.0']) {
    it(`offers only native evaluation for ${model}, including when metadata is absent`, () => {
      for (const data of [{}, TEXT_METADATA]) {
        const examples = buildModelApiExamples(model, data, BASE);
        assert.deepEqual(examples.map(({ id }) => id), ['systemone']);
        assert.equal(examples[0].endpoint, `${BASE}/v1/systemone`);
        assert.equal(examples[0].method, 'POST');
        assert.equal(examples[0].responseFormat, 'json');
        assert.equal(examples[0].note, 'systemone');
        assert.doesNotMatch(examples[0].request, /chat\/completions|\/responses|\/messages|anthropic-version/);
      }
    });
  }

  shellTest('the actual curl arguments carry native state/questions and a matching typed answer', () => {
    const [example] = buildModelApiExamples('jev-latest', TEXT_METADATA, BASE);
    const args = captureCurl(example);
    assert.deepEqual(args.slice(0, 3), ['--request', 'POST', `${BASE}/v1/systemone`]);
    assert.ok(args.includes('Authorization: Bearer YOUR_API_KEY'));
    assert.ok(args.includes('Content-Type: application/json'));
    const request = JSON.parse(args[args.indexOf('--data-raw') + 1]);
    assert.deepEqual(Object.keys(request).sort(), ['model', 'questions', 'state']);
    assert.equal(request.model, 'jev-latest');
    assert.equal(typeof request.state, 'string');
    assert.ok(request.state.length > 0);
    assert.deepEqual(Object.keys(request.questions), ['is_urgent']);
    assert.equal(request.questions.is_urgent.type, 'noul');
    assert.equal(typeof request.questions.is_urgent.instructions, 'string');
    assert.ok(request.questions.is_urgent.instructions.length > 0);

    const response = JSON.parse(example.response);
    assert.deepEqual(Object.keys(response).sort(), ['answers', 'model', 'usage']);
    assert.equal(response.model, 'jev-1.13.0');
    assert.deepEqual(Object.keys(response.answers), Object.keys(request.questions));
    assert.equal(response.answers.is_urgent.type, request.questions.is_urgent.type);
    assert.equal(typeof response.answers.is_urgent.noul, 'number');
    assert.ok(response.answers.is_urgent.noul >= 0 && response.answers.is_urgent.noul <= 1);
    assert.deepEqual(Object.keys(response.usage).sort(), ['input_tokens', 'output_tokens']);
    assert.ok(Number.isInteger(response.usage.input_tokens) && response.usage.input_tokens > 0);
    assert.ok(Number.isInteger(response.usage.output_tokens) && response.usage.output_tokens >= 0);
    // No messages, stream, max_tokens, choices or OpenAI token-counter fields are synthesized.
  });

  shellTest('preserves a configured model alias and deployment prefix with explicit System One metadata', () => {
    const model = `tenant/it's "urgent" $(printf INJECTED); \`printf INJECTED\``;
    const [example] = buildModelApiExamples(model, { supported_features: ['systemone'] }, `${BASE}/one-api/v1/`);
    const args = captureCurl(example);
    assert.equal(example.id, 'systemone');
    assert.equal(args[2], `${BASE}/one-api/v1/systemone`);
    assert.equal(JSON.parse(args[args.indexOf('--data-raw') + 1]).model, model);
  });

  it('keeps versioned responses consistent with a selected pinned model', () => {
    const [example] = buildModelApiExamples('typesafe/jev-1.13.0', {}, BASE);
    assert.equal(JSON.parse(example.response).model, 'jev-1.13.0');
  });

  it('does not infer native evaluation or chat from text modalities or a coincidental Jev substring', () => {
    assert.deepEqual(buildModelApiExamples('gpt-4o', TEXT_METADATA, BASE).map(({ id }) => id), ['chat', 'responses', 'messages']);
    for (const model of ['custom-chat', 'my-jev-latest-chat', 'jevish-chat']) {
      assert.deepEqual(buildModelApiExamples(model, TEXT_METADATA, BASE), []);
    }
    assert.equal(buildModelApiExamples('jina-embeddings-v3', {}, BASE)[0].id, 'embeddings');
    assert.equal(buildModelApiExamples('rerank-v4.0-pro', {}, BASE)[0].id, 'rerank');
  });
});
