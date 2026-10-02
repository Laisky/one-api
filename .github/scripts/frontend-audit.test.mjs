import assert from 'node:assert/strict';
import { test } from 'node:test';
import { verifyAudit } from './frontend-audit.mjs';

/** report creates synthetic audit evidence to test failure-closed validation. */
function report(overrides = {}) {
  return JSON.stringify({ type: 'auditSummary', data: { vulnerabilities: {
    info: 0, low: 0, moderate: 0, high: 0, critical: 0, ...overrides,
  } } });
}

test('audit accepts complete clean evidence', () => assert.ok(verifyAudit(report(), 0)));
for (const severity of ['info', 'low', 'moderate', 'high', 'critical']) {
  test(`audit rejects ${severity} advisories`, () => assert.throws(() => verifyAudit(report({ [severity]: 1 }), 0)));
}
for (const [name, output, status] of [
  ['network failure', '', 1], ['missing report', '{"type":"info"}', 0],
  ['truncated JSON', '{', 0], ['nonzero exit', report(), 1], ['terminated process', report(), null],
  ['duplicate summary', report() + '\n' + report(), 0],
  ['registry error', '{"type":"error"}\n' + report(), 0],
  ['contradictory advisory', '{"type":"auditAdvisory"}\n' + report(), 0],
  ['missing severity', report({ high: undefined }), 0], ['invalid severity', report({ high: '0' }), 0],
]) {
  test(`audit rejects ${name}`, () => assert.throws(() => verifyAudit(output, status)));
}
