import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const localeRoot = path.resolve('src/i18n/locales');
const supportedLanguages = ['en', 'es', 'fr', 'ja', 'zh'];

// collectLocaleFiles returns the sorted locale filenames for a language.
const collectLocaleFiles = (language) =>
  fs
    .readdirSync(path.join(localeRoot, language))
    .filter((filename) => fs.statSync(path.join(localeRoot, language, filename)).isFile())
    .sort();

// collectJsonKeyPaths returns sorted leaf paths from a locale JSON object.
const collectJsonKeyPaths = (value, prefix = '') => {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return Object.entries(value).flatMap(([key, child]) => collectJsonKeyPaths(child, prefix ? `${prefix}.${key}` : key));
  }

  return [prefix];
};

// getLineCount returns the number of text lines in a locale file.
const getLineCount = (content) => content.split('\n').length - (content.endsWith('\n') ? 1 : 0);

const referenceLanguage = supportedLanguages[0];
const referenceFiles = collectLocaleFiles(referenceLanguage);
const failures = [];

for (const language of supportedLanguages.slice(1)) {
  const files = collectLocaleFiles(language);
  const missing = referenceFiles.filter((filename) => !files.includes(filename));
  const extra = files.filter((filename) => !referenceFiles.includes(filename));

  for (const filename of missing) {
    failures.push(`${language} is missing ${filename}.`);
  }

  for (const filename of extra) {
    failures.push(`${language} has unexpected ${filename}.`);
  }
}

for (const filename of referenceFiles) {
  const byLanguage = new Map();

  for (const language of supportedLanguages) {
    const fullPath = path.join(localeRoot, language, filename);
    const content = fs.readFileSync(fullPath, 'utf8');
    byLanguage.set(language, {
      content,
      lineCount: getLineCount(content),
      keyPaths: filename.endsWith('.json') ? collectJsonKeyPaths(JSON.parse(content)).sort() : [],
    });
  }

  const lineCounts = supportedLanguages.map((language) => byLanguage.get(language).lineCount);
  if (new Set(lineCounts).size !== 1) {
    failures.push(`${filename} has mismatched line counts: ${supportedLanguages.map((language) => `${language}:${byLanguage.get(language).lineCount}`).join(', ')}.`);
  }

  if (!filename.endsWith('.json')) {
    continue;
  }

  const allKeyPaths = [...new Set(supportedLanguages.flatMap((language) => byLanguage.get(language).keyPaths))].sort();
  const keyCounts = supportedLanguages.map((language) => byLanguage.get(language).keyPaths.length);
  if (new Set(keyCounts).size !== 1) {
    failures.push(`${filename} has mismatched key counts: ${supportedLanguages.map((language) => `${language}:${byLanguage.get(language).keyPaths.length}`).join(', ')}.`);
  }

  for (const language of supportedLanguages) {
    const languageKeys = new Set(byLanguage.get(language).keyPaths);
    const missingKeys = allKeyPaths.filter((keyPath) => !languageKeys.has(keyPath));

    for (const keyPath of missingKeys) {
      failures.push(`${language}/${filename} is missing ${keyPath}.`);
    }
  }
}

if (failures.length > 0) {
  console.error('i18n locale alignment check failed:');
  for (const failure of failures) {
    console.error(`- ${failure}`);
  }
  process.exitCode = 1;
} else {
  console.log('i18n locale alignment check passed for en/es/fr/ja/zh.');
}
