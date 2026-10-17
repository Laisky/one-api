import { createHash } from 'node:crypto';
import { lstatSync, readFileSync, readdirSync, realpathSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repository = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const manifestPath = 'web/legacy/browser-release-inputs.json';
const themes = ['modern', 'air', 'berry'];

/** validateInputPath rejects private segments, credential files and noncanonical paths before filesystem access. */
function validateInputPath(name) {
  if (
    typeof name !== 'string' || !name.startsWith('web/') || name.includes('\\') ||
    name.split('/').some((part) =>
      !part ||
      (part.startsWith('.') && !(part === '.well-known' && /^web\/(modern|air|berry)\/public\/\.well-known(?:\/|$)/.test(name))) ||
      part === 'node_modules' || part === 'build' ||
      /^(?:private|credentials?|secrets?)(?:\.|$)|^id_(?:rsa|dsa|ecdsa|ed25519)(?:_sk)?(?:\.|$)|\.(?:pem|key|p12|pfx|keystore)$/i.test(part)
    )
  ) throw new Error('Invalid browser release input');
}

/** inputStat resolves a canonical input without following file or ancestor symlinks and returns its metadata. */
function inputStat(name, inputRoot = repository) {
  validateInputPath(name);
  let current = inputRoot;
  for (const part of name.split('/')) {
    current = path.join(current, part);
    if (lstatSync(current).isSymbolicLink()) throw new Error('Symbolic browser release input is not allowed');
  }
  return lstatSync(current);
}

/** readReleaseInput reads a safe regular source/build file without following symlinks or private paths. */
export function readReleaseInput(name, inputRoot = repository) {
  if (!inputStat(name, inputRoot).isFile()) throw new Error('Browser release input must be a regular file');
  return readFileSync(path.join(inputRoot, name));
}
const manifest = JSON.parse(readReleaseInput(manifestPath).toString('utf8'));

/** browserReleaseInputs discovers deterministic regular inputs from declared theme roots and explicit build/shared files without Git. */
export function browserReleaseInputs(theme) {
  if (!themes.includes(theme)) throw new Error('Unsupported browser release theme');
  if (!manifest || Array.isArray(manifest) || Object.keys(manifest).length !== themes.length || themes.some((name) => !Object.hasOwn(manifest, name)))
    throw new Error('Expected exactly the Modern, Air and Berry release input declarations');
  const declaration = manifest[theme];
  const roots = ['web/' + theme + '/src', 'web/' + theme + '/public'];
  if (
    !declaration || Array.isArray(declaration) || Object.keys(declaration).length !== 2 ||
    !Array.isArray(declaration.roots) || declaration.roots.length !== roots.length || roots.some((name) => !declaration.roots.includes(name)) ||
    !Array.isArray(declaration.files) || new Set(declaration.files).size !== declaration.files.length || !declaration.files.includes(manifestPath)
  ) throw new Error('Invalid browser release input declaration for ' + theme);
  const files = [...declaration.files];
  for (const name of files) {
    validateInputPath(name);
    if (!(name.startsWith('web/' + theme + '/') || name.startsWith('web/legacy/')) || roots.some((root) => name.startsWith(root + '/')))
      throw new Error('Expected an explicit theme configuration or shared browser input');
    if (!inputStat(name).isFile()) throw new Error('Browser release input must be a regular file');
  }
  /** visit collects public source paths after rejecting private names, special files and symlinks before reading their contents. */
  function visit(directory) {
    if (!inputStat(directory).isDirectory()) throw new Error('Browser release input root must be a directory');
    for (const entry of readdirSync(path.join(repository, directory), { withFileTypes: true })) {
      const name = directory + '/' + entry.name;
      validateInputPath(name);
      if (entry.isSymbolicLink()) throw new Error('Symbolic browser release input is not allowed');
      if (entry.isDirectory()) visit(name);
      else if (entry.isFile()) files.push(name);
      else throw new Error('Browser release input must be a regular file');
    }
  }
  for (const directory of roots) visit(directory);
  return files.sort();
}

/** captureInputs snapshots discovered input bytes and returns their immutable SHA256 build identity and per-file signatures. */
function captureInputs(theme, buildValues, read) {
  const hash = createHash('sha256');
  const signatures = new Map();
  hash.update(JSON.stringify({
    theme, node: process.versions.node, nodeEnvironment: process.env.NODE_ENV || 'production',
    platform: process.platform, architecture: process.arch, ...buildValues,
  }));
  for (const name of browserReleaseInputs(theme)) {
    const content = read(name);
    hash.update(name + ':' + content.length + ':');
    hash.update(content);
    signatures.set(name, createHash('sha256').update(content).digest('hex'));
  }
  return { release: `one-api-${theme}@sha256:${hash.digest('hex')}`, signatures };
}

/** browserRelease computes a deterministic identity from discovered source/build inputs and explicit public values without arbitrary environment or Git reads. */
export function browserRelease(theme, buildValues, read = readReleaseInput) {
  return captureInputs(theme, buildValues, read).release;
}

/** browserBuildContract returns one release and build guard sharing the same immutable source snapshot. */
export function browserBuildContract(theme, buildValues, read = readReleaseInput) {
  const snapshot = captureInputs(theme, buildValues, read);
  return { release: snapshot.release, plugin: releaseInputGuard(theme, snapshot, read) };
}

/** releaseInputGuard fails builds when actual first-party inputs escape the declared roots or change after the release snapshot. */
export function releaseInputGuard(theme, snapshot = captureInputs(theme, {}, readReleaseInput), read = readReleaseInput) {
  const inputs = new Set(snapshot.signatures.keys());
  const dependencyRoot = path.join(repository, 'web', theme, 'node_modules');
  let resolvedDependencyRoot = dependencyRoot;
  try {
    resolvedDependencyRoot = realpathSync(dependencyRoot);
  } catch {
    /* Dependency installation precedes a real build. */
  }
  /** verifyResolved permits only actual theme dependencies, internal virtual modules, and snapshotted regular first-party inputs. */
  function verifyResolved(id) {
    if (id.startsWith('\0')) return;
    const file = id.split('?')[0];
    if (!path.isAbsolute(file)) throw new Error('Unreviewed browser module input');
    const dependency = path.relative(resolvedDependencyRoot, file);
    if (dependency && !dependency.startsWith('..') && !path.isAbsolute(dependency)) {
      let actual;
      try { actual = realpathSync(file); } catch { throw new Error('Unreviewed browser dependency input'); }
      const resolved = path.relative(resolvedDependencyRoot, actual);
      if (resolved && !resolved.startsWith('..') && !path.isAbsolute(resolved)) return;
      throw new Error('Unreviewed browser dependency input');
    }
    const relative = path.relative(repository, file).replaceAll('\\', '/');
    if (!inputs.has(relative)) throw new Error('Unlisted browser build input; input is outside the release snapshot');
    readReleaseInput(relative);
  }
  /** verifySnapshot rejects changed source/config bytes and membership before public artifacts can be emitted. */
  function verifySnapshot() {
    const names = browserReleaseInputs(theme);
    if (names.length !== inputs.size || names.some((name) => !inputs.has(name)))
      throw new Error('Browser build inputs changed after release snapshot');
    for (const [name, expected] of snapshot.signatures) {
      if (createHash('sha256').update(read(name)).digest('hex') !== expected)
        throw new Error('Browser build input changed after release snapshot');
    }
  }
  return {
    name: 'oneapi-browser-release-inputs', apply: 'build', enforce: 'pre',
    /** buildStart verifies source/public completeness and the initial snapshot before output is generated. */
    buildStart() { verifySnapshot(); },
    /** generateBundle verifies unchanged input bytes and membership before writing any public artifact. */
    generateBundle() { verifySnapshot(); },
    /** resolveId validates resolved modules before loading while preserving resolver context and conditions. */
    async resolveId(source, importer, options = {}) {
      if (source.startsWith('\0')) return null;
      const resolved = await this.resolve(source, importer, { ...options, skipSelf: true });
      if (!resolved) return null;
      if (resolved.external) throw new Error('External browser module input is not approved');
      verifyResolved(resolved.id);
      return resolved;
    },
    /** load guards direct loader inputs before another plugin can read first-party source content. */
    load(id) { verifyResolved(id); return null; },
    /** transform verifies actual module inputs again while leaving approved dependencies and virtual modules unchanged. */
    transform(_code, id) { verifyResolved(id); return null; },
  };
}
