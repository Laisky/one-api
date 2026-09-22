import { MODEL_API_REVIEWED_ON, MODEL_API_SOURCES, resolveModelApiProfile } from './model-api-profiles';

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
  note?: 'upload' | 'video' | 'grokVideo' | 'realtime' | 'voice' | 'document' | 'clone' | 'systemone' | 'geminiLive' | 'diarization' | 'jinaOcr' | 'legacy';
  source?: string;
  reviewedOn?: string;
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

/** uploadExample builds multipart requests and returns representative JSON without overriding curl's boundary. */
function uploadExample(id: string, endpoint: string, model: string, fileField: string, response: unknown, fields: Record<string, string> = {}): ModelApiExample {
  return {
    id,
    method: 'POST',
    endpoint,
    request: formatCurl('POST', endpoint, [
      `--form-string ${quoteShellArgument(`model=${model}`)}`,
      `--form ${quoteShellArgument(fileField)}`,
      ...(id === 'image_edit' ? [`--form-string ${quoteShellArgument('prompt=Add a blue sky to this image.')}`] : []),
      ...Object.entries(fields).map(([key, value]) => `--form-string ${quoteShellArgument(`${key}=${value}`)}`),
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

/** websocketExample builds only an HTTP upgrade probe for the supplied native protocol; it does not send application frames. */
function websocketExample(model: string, base: string, gemini: boolean): ModelApiExample {
  const endpoint = `${base}/v1/realtime?model=${encodeURIComponent(model)}`;
  return {
    id: gemini ? 'gemini_live' : 'realtime', method: 'GET', endpoint,
    request: formatCurl('GET', endpoint, [
      '--http1.1 --include --no-buffer --max-time 10',
      ...['Connection: Upgrade', 'Upgrade: websocket', 'Sec-WebSocket-Version: 13', 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ=='].map((header) => `--header ${quoteShellArgument(header)}`),
    ]),
    response: 'HTTP/1.1 101 Switching Protocols\nConnection: Upgrade\nUpgrade: websocket\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=',
    responseFormat: 'http', note: gemini ? 'geminiLive' : 'realtime',
  };
}

/** examplesForProfile builds minimal gateway requests for a reviewed contract and returns no executable guess otherwise. */
function examplesForProfile(model: string, data: ModelApiMetadata, base: string): ModelApiExample[] {
  const name = model.toLowerCase().split('/').pop() || model.toLowerCase();
  const profile = resolveModelApiProfile(model, data);
  const prompt = 'A paper boat floating on a calm lake.';
  switch (profile.kind) {
    case 'unverified':
      return [];
    case 'chat':
      return chatExamples(model, base);
    case 'responses':
      return chatExamples(model, base).filter(({ id }) => id === 'responses');
    case 'systemone':
      return [jsonExample('systemone', `${base}/v1/systemone`, {
        model, state: 'The payment failed again. Please resolve this today.',
        questions: { is_urgent: { type: 'noul', instructions: 'Does this message request urgent action?' } },
      }, {
        model: /^jev-\d+\.\d+\.\d+$/.test(name) ? name : 'jev-1.13.0',
        answers: { is_urgent: { type: 'noul', noul: 0.92 } },
        usage: { input_tokens: 312, output_tokens: 48 },
      }, 'systemone')];
    case 'rerank':
      return [jsonExample('rerank', `${base}/v1/rerank`, {
        model, query: 'What is the capital of France?', documents: ['Paris is the capital of France.', 'Berlin is the capital of Germany.'], top_n: 1,
      }, { results: [{ index: 0, relevance_score: 0.98 }] })];
    case 'embeddings':
      // Use the batch form accepted by the reviewed embedding APIs and the
      // gateway converters. Never claim the shortened vector has real size.
      return [jsonExample('embeddings', `${base}/v1/embeddings`, { model, input: ['Hello, world!'] }, {
        object: 'list', model, data: [{ object: 'embedding', index: 0, embedding: [0.012, -0.034, 0.056] }], usage: { prompt_tokens: 4, total_tokens: 4 },
      })];
    case 'moderation':
      return [jsonExample('moderation', `${base}/v1/moderations`, { model, input: 'Hello, world!' }, {
        id: 'modr-example', model, results: [{ flagged: false, categories: { violence: false }, category_scores: { violence: 0.001 } }],
      })];
    case 'ocr':
      return [jsonExample('ocr', `${base}/api/paas/v4/layout_parsing`, { model, file: 'https://example.com/document.pdf' }, {
        md_results: '# Example document\n\nRecognized text.', layout_details: [],
      }, 'document')];
    case 'jina_ocr':
      return [jsonExample('jina_ocr', `${base}/v1/chat/completions`, {
        model, messages: [{ role: 'user', content: [
          { type: 'text', text: 'Convert this document to Markdown.' },
          { type: 'image_url', image_url: { url: 'https://example.com/document.png' } },
        ] }], stream: false,
      }, {
        id: 'chatcmpl-example', object: 'chat.completion', model,
        choices: [{ index: 0, message: { role: 'assistant', content: '# Example document\n\nRecognized text.' }, finish_reason: 'stop' }],
        usage: { prompt_tokens: 256, completion_tokens: 12, total_tokens: 268 },
      }, 'jinaOcr')];
    case 'clone':
      return [jsonExample('clone', `${base}/v1/voice/clones`, {
        model, voice_name: 'example-voice', file_id: 'YOUR_AUDIO_FILE_ID', text: 'The words spoken in the reference recording.', input: 'Hello, world!',
      }, { voice: 'example-voice-id', file_id: 'example-preview-file', request_id: 'example-request' }, 'clone')];
    case 'realtime':
    case 'gemini_live':
      return [websocketExample(model, base, profile.kind === 'gemini_live')];
    case 'diarization': {
      const example = uploadExample('diarization', `${base}/v1/audio/transcriptions`, model, 'file=@audio.wav', {
        text: 'Hello, world!', segments: [{ id: 'seg_0', start: 0, end: 1.2, speaker: 'A', text: 'Hello, world!' }],
      }, { response_format: 'diarized_json', chunking_strategy: 'auto' });
      return [{ ...example, note: 'diarization' }];
    }
    case 'transcription': {
      const fields: Record<string, string> = name.startsWith('glm-asr-') ? {} : { response_format: 'json' };
      const examples = [uploadExample('transcription', `${base}/v1/audio/transcriptions`, model, 'file=@audio.wav', { text: 'Hello, world!' }, fields)];
      // Groq's Whisper Turbo, GLM-ASR and OpenAI's newer transcribers do not
      // inherit Whisper-1's translation contract merely because of their name.
      if (name === 'whisper-1') examples.push(uploadExample('translation', `${base}/v1/audio/translations`, model, 'file=@audio.wav', { text: 'Hello, world!' }, { response_format: 'json' }));
      return examples;
    }
    case 'speech': {
      const example = jsonExample('speech', `${base}/v1/audio/speech`, {
        model, input: 'Hello, world!', voice: name === 'glm-tts' ? 'tongtong' : 'alloy', response_format: 'wav',
      }, {}, 'voice');
      return [{ ...example, request: `${example.request} \\\n  --output speech.wav`, response: 'HTTP/1.1 200 OK\nContent-Type: audio/wav\n\n<binary WAV audio saved to speech.wav>', responseFormat: 'http' }];
    }
    case 'video':
      // Both the gateway's per-second admission and Sora's JSON API use seconds.
      return [jsonExample('video', `${base}/v1/videos`, { model, prompt, seconds: '4', size: '1280x720' }, {
        id: 'video_example', object: 'video', model, status: 'queued', seconds: '4', size: '1280x720',
      }, 'video')];
    case 'grok_video': {
      const endpoint = `${base}/v1/videos/YOUR_REQUEST_ID`;
      const body = { model, prompt, duration: 5, aspect_ratio: '16:9', resolution: '720p' };
      const receipt = { request_id: 'YOUR_REQUEST_ID' };
      return [
        jsonExample('grok_video', `${base}/v1/videos/generations`, body, receipt, 'grokVideo'),
        jsonExample('grok_video_image', `${base}/v1/videos/generations`, {
          ...body, image: { url: 'https://example.com/input-image.png' },
        }, receipt, 'grokVideo'),
        {
          id: 'grok_video_status', method: 'GET', endpoint,
          request: formatCurl('GET', endpoint, []),
          response: JSON.stringify({ status: 'done', video: { url: 'https://vidgen.x.ai/example/video.mp4', duration: 5, respect_moderation: true }, model }, null, 2),
          responseFormat: 'json', note: 'grokVideo',
        },
      ];
    }
    case 'cogview':
      // The Zhipu image converter forwards model/prompt, not response_format.
      return [jsonExample('image', `${base}/v1/images/generations`, { model, prompt }, {
        created: 1700000000, data: [{ url: 'https://example.com/generated-image.png' }],
      })];
    case 'grok_image':
      return [jsonExample('image', `${base}/v1/images/generations`, { model, prompt, response_format: 'b64_json' }, {
        data: [{ b64_json: 'BASE64_ENCODED_IMAGE' }],
      })];
    case 'image': {
      const response = { created: 1700000000, data: [{ b64_json: 'BASE64_ENCODED_IMAGE' }] };
      const gptImage = !name.startsWith('dall-e-');
      const body: Record<string, unknown> = { model, prompt, n: 1 };
      // Honor only a size valid for the chosen wire contract, not arbitrary
      // provider pricing labels such as "1k" or "square".
      const allowedSizes = gptImage ? ['auto', '1024x1024', '1536x1024', '1024x1536'] : name === 'dall-e-3' ? ['1024x1024', '1792x1024', '1024x1792'] : ['256x256', '512x512', '1024x1024'];
      if (data.image_pricing?.default_size && allowedSizes.includes(data.image_pricing.default_size)) body.size = data.image_pricing.default_size;
      if (!gptImage) body.response_format = 'b64_json';
      const examples = [jsonExample('image', `${base}/v1/images/generations`, body, response)];
      if (gptImage || name === 'dall-e-2') examples.push(uploadExample('image_edit', `${base}/v1/images/edits`, model, 'image=@image.png', response, gptImage ? {} : { response_format: 'b64_json' }));
      return examples;
    }
    case 'completions':
      return [jsonExample('completions', `${base}/v1/completions`, { model, prompt: 'Say hello.', max_tokens: 32 }, {
        id: 'cmpl-example', object: 'text_completion', model, choices: [{ index: 0, text: 'Hello!', finish_reason: 'stop' }],
      }, 'legacy')];
  }
}

/** buildModelApiExamples returns sourced gateway templates for the selected model, or an empty list when its contract is unverified. */
export function buildModelApiExamples(model: string, data: ModelApiMetadata, baseUrl: string): ModelApiExample[] {
  const profile = resolveModelApiProfile(model, data);
  return examplesForProfile(model, data, normalizeApiBaseUrl(baseUrl)).map((example) => ({
    ...example,
    source: example.id === 'messages' ? MODEL_API_SOURCES.messages : example.id === 'responses' ? MODEL_API_SOURCES.responses : example.id === 'image_edit' ? MODEL_API_SOURCES.image_edit : profile.source,
    reviewedOn: profile.kind === 'grok_video' ? '2026-09-22' : MODEL_API_REVIEWED_ON,
  }));
}
