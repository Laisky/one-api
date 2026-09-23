import fs from 'node:fs';
import { pathToFileURL } from 'node:url';
import { resolveModelApiProfile } from '../web/modern/src/pages/models/model-api-profiles.ts';

const cohere = new Set(['embed-v4.0', 'embed-english-v3.0', 'embed-english-light-v3.0', 'embed-multilingual-v3.0', 'embed-multilingual-light-v3.0']);
const groq = new Set(['canopylabs/orpheus-v1-english', 'canopylabs/orpheus-arabic-saudi', 'whisper-large-v3', 'whisper-large-v3-turbo']);
const mistral = new Set(['voxtral-tts-2603', 'voxtral-mini-tts-2603', 'voxtral-mini-tts-latest', 'voxtral-mini-transcribe-2602', 'voxtral-mini-2602']);
const videos = new Set(['cogvideox-2', 'cogvideox-3', 'cogvideox-flash', 'viduq1-text', 'viduq1-image', 'viduq1-start-end', 'vidu2-image', 'vidu2-start-end', 'vidu2-reference']);
const images = new Set(['black-forest-labs/FLUX.1-schnell', 'black-forest-labs/FLUX.1.1-pro', 'black-forest-labs/FLUX-1.1-pro', 'black-forest-labs/FLUX.2-pro', 'black-forest-labs/FLUX.2-flex', 'Bytedance/Z-Image-Turbo', 'Tongyi-MAI/Z-Image-Turbo']);

/** classify distinguishes qualified channel contracts from mere example availability. */
export function classify(row) {
  if (!row || typeof row.model !== 'string' || !Number.isInteger(row.channel_type)) throw new TypeError('catalog row requires model and channel_type');
  const profile = resolveModelApiProfile(row.model, row);
  const channel = row.channel_type;
  const qualified = (channel === 35 && cohere.has(row.model)) || (channel === 29 && groq.has(row.model)) ||
    (channel === 28 && mistral.has(row.model)) || (channel === 16 && videos.has(row.model)) ||
    (channel === 58 && row.model === 'cogvideox-3') ||
    (channel === 44 && (images.has(row.model) || row.model === 'FunAudioLLM/CosyVoice2-0.5B'));
  let status = 'existing-example-not-requalified';
  let next = 'Retain existing behavior; example presence is not live endpoint or billing qualification.';
  if (qualified) {
    status = 'qualified-functional-and-ledger-tests';
    next = 'Use the documented standard subset; provider account access and paid live output were not tested.';
  } else if (profile.kind === 'unverified') {
    status = profile.reason === 'native' ? 'native-contract-and-billing-review-required' : 'no-reviewed-example';
    next = profile.reason === 'native'
      ? 'Review the native request/response, metering and standard conversion; do not infer unsupported from this label.'
      : 'Determine actual channel contract and pricing unit before adding a reviewed example or implementation.';
  }
  return { provider:row.provider, channel_type:channel, model:row.model, example:profile.kind,
    status, reference:profile.source, next_action:next };
}

/** audit produces deterministic per-channel rows; the same slug can have different provider protocols. */
export function audit(catalog) {
  if (!Array.isArray(catalog)) throw new TypeError('catalog must be an array');
  return catalog.map(classify).sort((a,b) => a.channel_type-b.channel_type || (a.model < b.model ? -1 : a.model > b.model ? 1 : 0));
}

/** toCSV encodes every value, including commas and quotes, without losing rows. */
export function toCSV(rows) {
  const columns = ['provider','channel_type','model','example','status','reference','next_action'];
  const quote = value => '"' + String(value ?? '').replaceAll('"', '""') + '"';
  return [columns.map(quote).join(','), ...rows.map(row => columns.map(key => quote(row[key])).join(','))].join('\n') + '\n';
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  if (process.argv.length !== 4) throw new Error('usage: node --experimental-strip-types scripts/model-protocol-audit.mjs catalog.json output.csv');
  const rows = audit(JSON.parse(fs.readFileSync(process.argv[2], 'utf8')));
  fs.writeFileSync(process.argv[3], toCSV(rows));
  const counts = {};
  for (const row of rows) counts[row.status] = (counts[row.status] ?? 0) + 1;
  console.log(JSON.stringify({channel_model_pairs:rows.length, unique_models:new Set(rows.map(row => row.model)).size, counts}));
}
