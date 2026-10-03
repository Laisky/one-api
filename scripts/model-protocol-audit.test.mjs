import { test } from 'node:test';
import assert from 'node:assert/strict';
import { audit, classify, toCSV } from './model-protocol-audit.mjs';

// These tests ensure a generated inventory never promotes a provider by name alone.
test('the same image slug requires channel-specific evidence', () => {
  const row = {model:'black-forest-labs/FLUX.2-pro',channel_type:44,provider:'siliconflow',supported_features:['gateway_siliconflow_image']};
  assert.equal(classify(row).status, 'qualified-functional-and-ledger-tests');
  assert.equal(classify({...row,channel_type:39,provider:'together.ai',supported_features:[]}).status, 'no-reviewed-example');
});
test('a pre-existing chat example is not a new verification', () => {
  assert.equal(classify({model:'gpt-4o',provider:'openai',channel_type:1}).status, 'existing-example-not-requalified');
});
test('native and unknown gaps stay visible', () => {
  assert.equal(classify({model:'veo-3.0-generate-001',provider:'gemini',channel_type:24}).status, 'native-contract-and-billing-review-required');
  assert.equal(classify({model:'not-in-any-reviewed-family',provider:'custom',channel_type:1}).status, 'no-reviewed-example');
});
test('the full catalog is retained in stable order and CSV values are escaped', () => {
  const rows = audit([{model:'z',provider:'"untrusted, label"',channel_type:35},{model:'embed-v4.0',provider:'Cohere',channel_type:35}]);
  assert.equal(rows.length,2);
  assert.equal(rows[0].model,'embed-v4.0');
  assert.match(toCSV(rows),/""untrusted, label""/);
  assert.throws(() => audit({}),TypeError);
  assert.throws(() => classify({model:'x'}),TypeError);
});
