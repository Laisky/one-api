import { lstatSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { readReleaseInput, releaseInputGuard } from '../../web/legacy/browser-release.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const manifestPath = 'web/legacy/browser-release-inputs.json';
const themes = ['modern', 'air', 'berry'];

/** readManifest returns the reviewed lists after rejecting unsupported themes, malformed lists and duplicate entries. */
function readManifest() {
  const manifest = JSON.parse(readReleaseInput(manifestPath).toString('utf8'));
  if (!manifest || Array.isArray(manifest) || Object.keys(manifest).length !== themes.length)
    throw new Error('Expected exactly the Modern, Air and Berry release input lists');
  for (const theme of themes) {
    const files = manifest[theme];
    if (!Array.isArray(files) || files.some((name) => typeof name !== 'string') || new Set(files).size !== files.length)
      throw new Error('Invalid or duplicate browser release inputs for ' + theme);
    for (const name of files) validateThemePath(theme, name);
  }
  return manifest;
}

/** validateThemePath accepts only explicit canonical paths for the selected theme or shared legacy helpers. */
function validateThemePath(theme, name) {
  if (
    typeof name !== 'string' ||
    name.includes('\\') ||
    name.split('/').some((part) => !part || part === '.' || part === '..') ||
    !(name.startsWith('web/' + theme + '/') || name.startsWith('web/legacy/'))
  )
    throw new Error('Expected a canonical input path for ' + theme);
}

/** sourceInputs enumerates source/public paths without following symlinks or reading unreviewed file contents. */
function sourceInputs(directory) {
  const full = path.join(root, directory);
  if (lstatSync(full).isSymbolicLink()) throw new Error('Symbolic browser release input is not allowed');
  return readdirSync(full, { withFileTypes: true }).flatMap((entry) => {
    if (entry.isSymbolicLink()) throw new Error('Symbolic browser release input is not allowed');
    const name = directory + '/' + entry.name;
    return entry.isDirectory() ? sourceInputs(name) : [name];
  });
}

/** checkManifest fails on unlisted source/public inputs, then applies the unchanged production build guard to every theme. */
function checkManifest(manifest) {
  const missing = themes.flatMap((theme) => {
    const reviewed = new Set(manifest[theme]);
    return ['src', 'public'].flatMap((directory) => sourceInputs('web/' + theme + '/' + directory).filter((name) => !reviewed.has(name)));
  });
  if (missing.length)
    throw new Error('Unlisted browser build inputs; review and explicitly add these paths:\n' + missing.sort().join('\n'));
  for (const theme of themes) releaseInputGuard(theme).buildStart();
  console.log('Reviewed browser release inputs cover Modern, Air and Berry.');
}

/** updateManifest validates the complete requested edit before writing only explicitly added or removed paths. */
function updateManifest(manifest, action, theme, files) {
  if (!themes.includes(theme) || !files.length) throw new Error('Specify a supported theme and at least one explicit file path');
  const reviewed = new Set(manifest[theme]);
  for (const name of files) {
    validateThemePath(theme, name);
    if (action === 'add') {
      readReleaseInput(name);
      reviewed.add(name);
    } else {
      if (!reviewed.has(name)) throw new Error('Cannot remove an input that is not reviewed: ' + name);
      if (name === manifestPath) throw new Error('The release manifest must remain part of every release identity');
      reviewed.delete(name);
    }
  }
  manifest[theme] = [...reviewed].sort();
  writeFileSync(path.join(root, manifestPath), JSON.stringify(manifest, null, 2) + '\n');
  console.log('Updated ' + theme + ' inputs. Review the manifest diff, then run this command with check.');
}

/** main runs a read-only check by default or a deliberate add/remove operation; builds never invoke updates. */
function main() {
  const [action = 'check', theme, ...files] = process.argv.slice(2);
  if (!['check', 'add', 'remove'].includes(action) || (action === 'check' && theme))
    throw new Error('Usage: node .github/scripts/browser-release-inputs.mjs [check | add THEME PATH... | remove THEME PATH...]');
  const manifest = readManifest();
  if (action === 'check') checkManifest(manifest);
  else updateManifest(manifest, action, theme, files);
}

try {
  main();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
