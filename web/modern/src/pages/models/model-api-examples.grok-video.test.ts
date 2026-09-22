import { execFileSync } from 'node:child_process';
import { describe, expect, it } from 'vitest';
import { buildModelApiExamples } from './model-api-examples';
import { MODEL_API_SOURCES, resolveModelApiProfile } from './model-api-profiles';

const models = ['grok-imagine-video', 'grok-imagine-video-2026-01-20', 'grok-imagine-video-1.5', 'grok-imagine-video-1.5-preview', 'grok-imagine-video-1.5-2026-05-30'];

describe('Grok video gateway contract', () => {
  it.each(models)('provides native creation and polling for %s', (model) => {
    const examples = buildModelApiExamples(model, { output_modalities: ['video'] }, 'https://gateway.example/proxy/v1/');
    expect(resolveModelApiProfile(model, {}).kind).toBe('grok_video');
    expect(examples.map(({ id }) => id)).toEqual(['grok_video', 'grok_video_image', 'grok_video_status']);
    expect(examples[0].endpoint).toBe('https://gateway.example/proxy/v1/videos/generations');
    expect(JSON.parse(examples[0].response)).toEqual({ request_id: 'YOUR_REQUEST_ID' });
    expect(examples[2].method).toBe('GET');
    expect(JSON.parse(examples[2].response)).toMatchObject({ status: 'done', model, video: { duration: 5 } });
    for (const example of examples) {
      expect(example.source).toBe(MODEL_API_SOURCES.grok_video);
      expect(example.reviewedOn).toBe('2026-09-22');
      expect(example.request).not.toMatch(/chat\/completions|\/content/);
      expect(example.note).toBe('grokVideo');
    }
  });

  it('does not infer a protocol from an unknown future Grok video ID', () => {
    expect(buildModelApiExamples('grok-unverified-video', {}, 'https://example.com')).toEqual([]);
  });

  it.skipIf(process.platform === 'win32')('executes the displayed curl through a fake shell command without networking', () => {
    for (const example of buildModelApiExamples('grok-imagine-video-1.5', {}, 'https://gateway.example')) {
      const args = execFileSync('sh', ['-c', `curl() { printf '%s\\0' "$@"; }\n${example.request}`], { encoding: 'utf8' }).split('\0').slice(0, -1);
      expect(args).toContain('Authorization: Bearer YOUR_API_KEY');
      if (example.method === 'POST') {
        const body = JSON.parse(args[args.indexOf('--data-raw') + 1]);
        expect(body).toMatchObject({ model: 'grok-imagine-video-1.5', duration: 5, resolution: '720p', aspect_ratio: '16:9' });
        expect(body).not.toHaveProperty('seconds');
        expect(body).not.toHaveProperty('size');
        if (example.id === 'grok_video_image') expect(body.image.url).toBe('https://example.com/input-image.png');
      } else {
        expect(args).not.toContain('--data-raw');
        expect(args).toContain('https://gateway.example/v1/videos/YOUR_REQUEST_ID');
      }
    }
  });
});
