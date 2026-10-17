import { browserReleaseInputs, releaseInputGuard } from '../../web/legacy/browser-release.mjs';

/** main validates declared browser inputs without rewriting source policy or requiring Git metadata. */
function main() {
  const [action = 'check', ...extra] = process.argv.slice(2);
  if (action !== 'check' || extra.length)
    throw new Error('Usage: node .github/scripts/browser-release-inputs.mjs [check]; source files are discovered from declared roots');
  for (const theme of ['modern', 'air', 'berry']) {
    browserReleaseInputs(theme);
    releaseInputGuard(theme).buildStart();
  }
  console.log('Declared browser release inputs cover Modern, Air and Berry.');
}

try { main(); } catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
