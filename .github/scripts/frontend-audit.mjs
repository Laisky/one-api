import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

/** verifyAudit requires one complete, zero-advisory report and a successful process. */
export function verifyAudit(output, status) {
  const entries = output.split('\n').filter(line => line.trim()).map(line => JSON.parse(line));
  assert.ok(entries.length > 0, 'The audit returned no evidence');
  assert.ok(!entries.some(entry => entry.type === 'error'), 'The registry audit returned an error');
  const summaries = entries.filter(entry => entry.type === 'auditSummary');
  assert.equal(summaries.length, 1, 'The audit must return exactly one summary');
  const counts = summaries[0].data.vulnerabilities;
  for (const severity of ['info', 'low', 'moderate', 'high', 'critical']) {
    assert.ok(Number.isInteger(counts[severity]) && counts[severity] >= 0, `Invalid ${severity} count`);
    assert.equal(counts[severity], 0, `Unresolved ${severity} dependency advisories`);
  }
  assert.ok(!entries.some(entry => entry.type === 'auditAdvisory'), 'Advisory details contradict the summary');
  assert.equal(status, 0, 'The audit process did not complete successfully');
  return summaries[0].data;
}

/** runAudit audits development and runtime dependencies and saves the raw evidence. */
export function runAudit(theme) {
  assert.ok(['modern', 'air', 'berry'].includes(theme), 'Unknown frontend theme');
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
  const result = spawnSync('yarn', ['audit', '--json', '--non-interactive'], {
    cwd: path.join(root, 'web', theme), encoding: 'utf8', timeout: 180000, maxBuffer: 32 * 1024 * 1024,
  });
  if (result.error) throw result.error;
  const report = path.join(process.env.RUNNER_TEMP || path.join(root, 'web', theme), `${theme}-audit.jsonl`);
  writeFileSync(report, result.stdout || '');
  const summary = verifyAudit(result.stdout || '', result.status);
  console.log(`${theme}: ${JSON.stringify(summary)}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  runAudit(process.argv[2]);
}
