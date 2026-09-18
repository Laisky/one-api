/** ModelApiMetadata contains the catalog capabilities needed to choose usage examples. */
export interface ModelApiMetadata {
  input_modalities?: string[];
  output_modalities?: string[];
  supported_features?: string[];
  embedding_pricing?: unknown;
  video_pricing?: unknown;
  image_pricing?: { default_size?: string };
}

/** ModelApiExample describes a gateway endpoint, a shell-safe request, and illustrative output. */
export interface ModelApiExample {
  id: string;
  method: 'GET' | 'POST';
  endpoint: string;
  request: string;
  response: string;
  responseFormat: 'json' | 'http';
  note?: 'upload' | 'video' | 'realtime' | 'voice' | 'document' | 'clone' | 'systemone';
}

const FALLBACK_BASE_URL = 'https://your-one-api.example';
const AUTH_HEADER = 'Authorization: Bearer YOUR_API_KEY';

/** quoteShellArgument returns one POSIX shell argument without expanding its contents. */
export function quoteShellArgument(value: string): string {
  return `'${value.replace(/'/g, `'"'"'`)}'`;
}

/** normalizeApiBaseUrl preserves deployment prefixes, rejects credential-bearing URLs, and avoids API suffix duplication. */
export function normalizeApiBaseUrl(value: string): string {
  try {
    const url = new URL(value);
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return FALLBACK_BASE_URL;
    url.search = '';
    url.hash = '';
    const prefix = url.pathname.replace(/\/+$/, '').replace(/\/v1$/, '');
    return `${url.origin}${prefix}`;
  } catch {
    return FALLBACK_BASE_URL;
  }
}

/** formatCurl returns a multiline curl command from already separated argument strings. */
function formatCurl(method: ModelApiExample['method'], endpoint: string, arguments_: string[]): string {
  return [
    `curl --request ${method} ${quoteShellArgument(endpoint)}`,
    `  --header ${quoteShellArgument(AUTH_HEADER)}`,
    ...arguments_.map((argument) => `  ${argument}`),
  ].join(' \\\n');
}

/** jsonExample builds a POST example using JSON encoding before shell quoting. */
function jsonExample(
  id: string,
  endpoint: string,
  body: Record<string, unknown>,
  response: unknown,
  note?: ModelApiExample['note']
): ModelApiExample {
  return {
    id,
    method: 'POST',
    endpoint,
    request: formatCurl('POST', endpoint, [
      `--header ${quoteShellArgument('Content-Type: application/json')}`,
      `--data-raw ${quoteShellArgument(JSON.stringify(body, null, 2))}`,
    ]),
    response: JSON.stringify(response, null, 2),
    responseFormat: 'json',
    note,
  };
}

/** uploadExample builds multipart requests without overriding curl's boundary-bearing Content-Type. */
function uploadExample(id: string, endpoint: string, model: string, fileField: string, response: unknown): ModelApiExample {
  return {
    id,
    method: 'POST',
    endpoint,
    request: formatCurl('POST', endpoint, [
      `--form-string ${quoteShellArgument(`model=${model}`)}`,
      `--form ${quoteShellArgument(fileField)}`,
      ...(id === 'image_edit' ? [`--form-string ${quoteShellArgument('prompt=Add a blue sky to this image.')}`] : []),
      ...(id === 'transcription' || id === 'translation' ? [`--form-string ${quoteShellArgument('response_format=json')}`] : []),
    ]),
    response: JSON.stringify(response, null, 2),
    responseFormat: 'json',
    note: 'upload',
  };
}

/** chatExamples returns the three gateway conversation formats for the selected catalog model. */
function chatExamples(model: string, base: string): ModelApiExample[] {
  const chat = jsonExample(
    'chat',
    `${base}/v1/chat/completions`,
    { model, messages: [{ role: 'user', content: 'Say hello.' }], stream: false },
    {
      id: 'chatcmpl-example',
      object: 'chat.completion',
      model,
      choices: [{ index: 0, message: { role: 'assistant', content: 'Hello!' }, finish_reason: 'stop' }],
      usage: { prompt_tokens: 3, completion_tokens: 2, total_tokens: 5 },
    }
  );
  const responses = jsonExample(
    'responses',
    `${base}/v1/responses`,
    { model, input: 'Say hello.', stream: false },
    {
      id: 'resp_example',
      object: 'response',
      model,
      status: 'completed',
      output: [{ id: 'msg_example', type: 'message', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: 'Hello!', annotations: [] }] }],
      usage: { input_tokens: 3, output_tokens: 2, total_tokens: 5 },
    }
  );
  const messages = jsonExample(
    'messages',
    `${base}/v1/messages`,
    { model, max_tokens: 1024, messages: [{ role: 'user', content: 'Say hello.' }], stream: false },
    {
      id: 'msg_example',
      type: 'message',
      role: 'assistant',
      model,
      content: [{ type: 'text', text: 'Hello!' }],
      stop_reason: 'end_turn',
      stop_sequence: null,
      usage: { input_tokens: 3, output_tokens: 2 },
    }
  );
  // These are gateway requests, not direct calls to any provider's API.
  messages.request = messages.request.replace(
    '  --header',
    `  --header ${quoteShellArgument('anthropic-version: 2023-06-01')} \\\n  --header`
  );
  return model.toLowerCase().includes('codex') ? [responses, chat, messages] : [chat, responses, messages];
}

/** buildModelApiExamples chooses gateway examples from capabilities and known non-chat model families. */
export function buildModelApiExamples(model: string, data: ModelApiMetadata, baseUrl: string): ModelApiExample[] {
  const base = normalizeApiBaseUrl(baseUrl);
  const name = model.toLowerCase().split('/').pop() || model.toLowerCase();
  const output = data.output_modalities ?? [];
  const features = data.supported_features ?? [];

  // Jev is a typed evaluator, even though its catalog modalities are text/text.
  // Native contract: https://docs.typesafe.ai/api (checked 2026-09-18).
  // Aliases resolve to a versioned response model: https://docs.typesafe.ai/models.
  if (/^jev(?:-|$)/.test(name) || features.includes('systemone')) {
    const responseModel = /^jev-\d+\.\d+\.\d+/.test(name) ? name : 'jev-1.13.0';
    return [jsonExample('systemone', `${base}/v1/systemone`, {
      model,
      state: 'The payment failed again. Please resolve this today.',
      questions: {
        is_urgent: { type: 'noul', instructions: 'Does this message request urgent action?' },
      },
    }, {
      model: responseModel,
      answers: { is_urgent: { type: 'noul', noul: 0.92 } },
      usage: { input_tokens: 312, output_tokens: 48 },
    }, 'systemone')];
  }

  // Specialized tasks take precedence over multimodal pricing: embeddings may
  // have image/audio/video prices without supporting media generation.
  if (/rerank/.test(name) || features.includes('rerank')) {
    return [jsonExample('rerank', `${base}/v1/rerank`, {
      model, query: 'What is the capital of France?', documents: ['Paris is the capital of France.', 'Berlin is the capital of Germany.'], top_n: 1,
    }, { results: [{ index: 0, relevance_score: 0.98 }] })];
  }
  if (data.embedding_pricing || /embed|^text2vec|^bge-|^e5-/.test(name) || features.includes('embeddings')) {
    return [jsonExample('embeddings', `${base}/v1/embeddings`, { model, input: 'Hello, world!' }, {
      object: 'list', model, data: [{ object: 'embedding', index: 0, embedding: [0.012, -0.034, 0.056] }], usage: { prompt_tokens: 4, total_tokens: 4 },
    })];
  }
  if (/moderation/.test(name)) {
    return [jsonExample('moderation', `${base}/v1/moderations`, { model, input: 'Hello, world!' }, {
      id: 'modr-example', model, results: [{ flagged: false, categories: { violence: false }, category_scores: { violence: 0.001 } }],
    })];
  }
  if (name === 'glm-ocr') {
    return [jsonExample('ocr', `${base}/api/paas/v4/layout_parsing`, { model, file: 'https://example.com/document.pdf' }, {
      md_results: '# Example document\n\nRecognized text.', layout_details: [],
    }, 'document')];
  }
  if (name === 'glm-tts-clone') {
    return [jsonExample('clone', `${base}/v1/voice/clones`, {
      model, voice_name: 'example-voice', file_id: 'YOUR_AUDIO_FILE_ID', text: 'The words spoken in the reference recording.', input: 'Hello, world!',
    }, { voice: 'example-voice-id', file_id: 'example-preview-file', request_id: 'example-request' }, 'clone')];
  }
  if (/realtime/.test(name)) {
    const endpoint = `${base}/v1/realtime?model=${encodeURIComponent(model)}`;
    return [{
      id: 'realtime', method: 'GET', endpoint,
      request: formatCurl('GET', endpoint, [
        '--http1.1 --include --no-buffer --max-time 10',
        ...['OpenAI-Beta: realtime=v1', 'Connection: Upgrade', 'Upgrade: websocket', 'Sec-WebSocket-Version: 13', 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ=='].map((header) => `--header ${quoteShellArgument(header)}`),
      ]),
      response: 'HTTP/1.1 101 Switching Protocols\nConnection: Upgrade\nUpgrade: websocket\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=',
      responseFormat: 'http', note: 'realtime',
    }];
  }
  if (/whisper|transcrib|(^|[-_])asr([-_]|$)/.test(name)) {
    const examples = [uploadExample('transcription', `${base}/v1/audio/transcriptions`, model, 'file=@audio.wav', { text: 'Hello, world!' })];
    if (/^whisper/.test(name)) examples.push(uploadExample('translation', `${base}/v1/audio/translations`, model, 'file=@audio.wav', { text: 'Hello, world!' }));
    return examples;
  }
  if (/tts|^speech-/.test(name) || (output.length === 1 && output[0] === 'audio')) {
    const example = jsonExample('speech', `${base}/v1/audio/speech`, {
      model, input: 'Hello, world!', voice: /^(tts-1|gpt.*tts)/.test(name) ? 'alloy' : 'YOUR_VOICE_ID', response_format: 'wav',
    }, {}, 'voice');
    return [{ ...example, request: `${example.request} \\\n  --output speech.wav`, response: 'HTTP/1.1 200 OK\nContent-Type: audio/wav\n\n<binary WAV audio saved to speech.wav>', responseFormat: 'http' }];
  }
  if (data.video_pricing || output.includes('video') || /^(sora|veo|cogvideo|wan.*t2v|kling)/.test(name)) {
    return [jsonExample('video', `${base}/v1/videos`, { model, prompt: 'A paper boat floating on a calm lake.' }, {
      id: 'video_example', object: 'video', model, status: 'queued',
    }, 'video')];
  }
  if (data.image_pricing || output.includes('image') || /^(dall-e|gpt-image|imagen|cogview|flux|stable-diffusion|sdxl)/.test(name)) {
    const response = { created: 1700000000, data: [{ b64_json: 'BASE64_ENCODED_IMAGE' }] };
    if (/(^|[-_])edit([-_]|$)/.test(name)) {
      return [uploadExample('image_edit', `${base}/v1/images/edits`, model, 'image=@image.png', response)];
    }
    const body: Record<string, unknown> = { model, prompt: 'A paper boat floating on a calm lake.', n: 1 };
    if (data.image_pricing?.default_size) body.size = data.image_pricing.default_size;
    // GPT Image returns base64 without accepting the legacy response_format parameter.
    if (!/^gpt-image/.test(name)) body.response_format = 'b64_json';
    return [jsonExample('image', `${base}/v1/images/generations`, body, response)];
  }
  if (/^text-(davinci|curie|babbage|ada)|^(davinci|babbage)-002$|gpt-3\.5-turbo-instruct/.test(name)) {
    return [jsonExample('completions', `${base}/v1/completions`, { model, prompt: 'Say hello.', max_tokens: 32 }, {
      id: 'cmpl-example', object: 'text_completion', model, choices: [{ index: 0, text: 'Hello!', finish_reason: 'stop' }],
    })];
  }

  // Input modalities and per-call/audio pricing alone are not endpoint evidence.
  // Unknown/custom chat aliases still receive a useful gateway request example.
  return chatExamples(model, base);
}
