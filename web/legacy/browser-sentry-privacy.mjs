const privateErrors = new WeakMap();
const MAX_CAUSES = 10;
const MAX_STRING = 32_768;
const PRIVATE_FIELDS =
  /^(?:authorization|proxyauthorization|(?:http|request|response)?headers|cookies?|setcookie|(?:api|sso|session|access|refresh|id)(?:tokens?|keys?)|tokens?|jwt|bearer|password|passwd|secret|clientsecret|privatekey|localstorage|prompts?|completions?|messages|chathistory|history|arguments|args|(?:tool|mcp)(?:inputs?|arguments|results?)|structuredcontent|raw(?:response|request|payload)|content|text|answer|commandtemplate|filecontent|requestbody|responsebody|body|inputs?|outputs?|private|user(?:id|name|email|identity|metadata|prefix)|account(?:id|name|email|metadata)?|session(?:id|user)?|username|email|phone|ipaddress|profile|user)$/i;
const URL_FIELDS = new Set(['url', 'filename', 'abs_path', 'code_file', 'from', 'to', 'referrer', 'referer']);
const PRIVATE_CONSOLE =
  /^(Failed to parse (?:SSE data|config JSON|channel config|localStorage|system status):?|Failed to (?:read|save|parse) localStorage:?|Error (?:saving|clearing) option:)/;

/** ownValue reads an own data property without invoking getters and returns its value. */
function ownValue(value, key) {
  if (!value || (typeof value !== 'object' && typeof value !== 'function')) return undefined;
  const descriptor = Object.getOwnPropertyDescriptor(value, key);
  return descriptor && 'value' in descriptor ? descriptor.value : undefined;
}

/** arrayValues snapshots a bounded array using only data descriptors, without invoking element getters. */
function arrayValues(value, limit) {
  if (!Array.isArray(value)) return [];
  const length = ownValue(value, 'length');
  if (!Number.isInteger(length) || length < 0) return [];
  const items = [];
  for (let index = 0; index < Math.min(length, limit); index++) items.push(ownValue(value, String(index)));
  return items;
}

/** markPrivateError annotates an error with a controlled summary without changing UI messages or stacks. */
export function markPrivateError(error, summary, expected = false) {
  if (error && (typeof error === 'object' || typeof error === 'function') && typeof summary === 'string' && summary) {
    privateErrors.set(error, { summary: sanitizeText(summary).slice(0, 160), expected: expected === true });
  }
  return error;
}

/** privateResponseError creates an expected response error while retaining its original UI message. */
export function privateResponseError(message, summary) {
  return markPrivateError(new Error(message), summary, true);
}

/** privateChain reads bounded acyclic linked causes and aggregate members for annotated message replacements. */
function privateChain(error, includeAggregates = true) {
  const redactions = [];
  const seen = new WeakSet();
  const pending = [error];
  for (let depth = 0; pending.length && depth < MAX_CAUSES; depth++) {
    error = pending.shift();
    if (!error || (typeof error !== 'object' && typeof error !== 'function') || seen.has(error)) continue;
    seen.add(error);
    const annotation = privateErrors.get(error);
    const message = ownValue(error, 'message');
    if (annotation && typeof message === 'string') redactions.push({ message, ...annotation });
    const cause = ownValue(error, 'cause');
    if (cause) pending.push(cause);
    const errors = includeAggregates ? ownValue(error, 'errors') : undefined;
    for (const member of arrayValues(errors, MAX_CAUSES)) if (pending.length < MAX_CAUSES) pending.push(member);
  }
  return redactions;
}

/** isExpectedRequestError recognizes an annotated expected error or its bounded linked cause. */
export function isExpectedRequestError(error) {
  try {
    return privateChain(error, false).some((entry) => entry.expected);
  } catch {
    return false;
  }
}

/** sanitizeURL removes URL credentials, query strings and fragments while retaining useful source paths. */
export function sanitizeURL(value) {
  if (typeof value !== 'string') return value;
  value = value.slice(0, MAX_STRING);
  if (/^data:/i.test(value)) return '[inline source]';
  if (/^blob:/i.test(value)) {
    const clean = sanitizeURL(value.slice(5));
    try {
      const url = new URL(clean);
      // Real browser Blob URLs have an opaque UUID, rather than user-authored source content.
      return /^\/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(url.pathname)
        ? 'blob:' + clean
        : 'blob:' + url.origin;
    } catch {
      return '[opaque source]';
    }
  }
  const protocolRelative = value.startsWith('//');
  if (!protocolRelative && !/^[a-z][a-z0-9+.-]*:\/\//i.test(value)) return value.split(/[?#]/, 1)[0];
  try {
    const url = new URL(protocolRelative ? 'https:' + value : value);
    url.username = '';
    url.password = '';
    url.search = '';
    url.hash = '';
    return protocolRelative ? url.toString().replace(/^https:/, '') : url.toString();
  } catch {
    return '[invalid URL]';
  }
}

/** sanitizeText masks recognizable credential shapes while preserving ordinary diagnostic text. */
export function sanitizeText(value) {
  return value
    .slice(0, MAX_STRING)
    .replace(/\b(?:https?|wss?):\/\/[^\s"'<>]+/gi, (match) => {
      const tail = match.match(/[),.;\]]+$/)?.[0] ?? '';
      return sanitizeURL(tail ? match.slice(0, -tail.length) : match) + tail;
    })
    .replace(/\b(Bearer|Basic)\s+[^\s,;"']+/gi, '$1 [Filtered]')
    .replace(/\bsk-[A-Za-z0-9_-]{6,}/g, '[Filtered]')
    .replace(/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g, '[Filtered]')
    .replace(
      /\b((?:api|sso|session|access|refresh|id)[_-]?(?:tokens?|keys?)|token|jwt|password|secret|authorization|cookie)\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)/gi,
      '$1=[Filtered]'
    );
}

/** redactMessage replaces only specifically annotated full, prefixed or SDK-truncated private messages. */
function redactMessage(value, redactions) {
  for (const { message, summary } of redactions) {
    if (value === message) return summary;
    if (message && value.includes(message)) value = value.replaceAll(message, summary);
    if (message.length >= 24 && value.endsWith('...')) {
      const start = value.indexOf(message.slice(0, 24));
      if (start >= 0 && message.startsWith(value.slice(start, -3))) value = value.slice(0, start) + summary;
    }
  }
  return sanitizeText(value);
}

/** normalizedKey maps common snake, kebab and camel credential aliases to a stable comparison key. */
function normalizedKey(key) {
  return key.replace(/[_-]/g, '').toLowerCase();
}

/** errorName reads a bounded prototype chain of name data properties without invoking getters. */
function errorName(error) {
  for (let depth = 0; error && depth < 4; depth++, error = Object.getPrototypeOf(error)) {
    const name = ownValue(error, 'name');
    if (typeof name === 'string') return sanitizeText(name);
  }
  return 'Error';
}

/** cloneData copies bounded metadata without getters, cycles or known credential and payload fields. */
function cloneData(
  value,
  redactions,
  key = '',
  parent = '',
  depth = 0,
  state = { nodes: 16_384, entries: 32_768, characters: 131_072, seen: new WeakSet() }
) {
  if (--state.nodes < 0) return '[Truncated]';
  const normalized = normalizedKey(key);
  if (
    PRIVATE_FIELDS.test(normalized) ||
    ['vars', 'contextline', 'precontext', 'postcontext'].includes(normalized) ||
    (normalized === 'data' && /^(request|response|config)$/i.test(parent)) ||
    (/^(document|variables)$/.test(normalized) && parent === 'graphql')
  )
    return undefined;
  if (typeof value === 'string') {
    const clean = URL_FIELDS.has(key) ? sanitizeURL(value) : redactMessage(value, redactions);
    const available = Math.max(0, state.characters);
    state.characters -= clean.length;
    return clean.length <= available ? clean : clean.slice(0, available) + '[Truncated]';
  }
  if (!value || typeof value !== 'object') return ['function', 'bigint', 'symbol'].includes(typeof value) ? undefined : value;
  if (depth > 12 || state.seen.has(value)) return '[Truncated]';
  state.seen.add(value);
  if (value instanceof Error) {
    const message = ownValue(value, 'message');
    return { name: errorName(value), message: typeof message === 'string' ? redactMessage(message, privateChain(value)) : 'Error' };
  }
  if (Array.isArray(value)) {
    const items = [];
    const length = ownValue(value, 'length');
    for (let index = 0; index < length && state.nodes > 0 && state.entries > 0; index++) {
      if (--state.entries < 0) break;
      items.push(cloneData(ownValue(value, String(index)), redactions, '', normalized, depth + 1, state));
    }
    return items;
  }
  const clean = {};
  let count = 0;
  for (const childKey in value) {
    if (!Object.prototype.hasOwnProperty.call(value, childKey)) continue;
    if (count++ >= 200 || --state.entries < 0 || state.nodes <= 0 || state.characters <= 0) break;
    if (childKey.length > 256) continue;
    const descriptor = Object.getOwnPropertyDescriptor(value, childKey);
    if (!descriptor || !('value' in descriptor)) continue;
    const child = cloneData(descriptor.value, redactions, childKey, normalized, depth + 1, state);
    if (child !== undefined) Object.defineProperty(clean, childKey, { value: child, enumerable: true, writable: true, configurable: true });
  }
  return clean;
}

/** consoleMessage rebuilds console diagnostics from sanitized arguments rather than preformatted private objects. */
function consoleMessage(breadcrumb, input, redactions) {
  const args = arrayValues(input, 50);
  const first = typeof args[0] === 'string' ? args[0] : '';
  const privatePrefix = first.match(PRIVATE_CONSOLE)?.[0];
  if (privatePrefix) return privatePrefix;
  const message = ownValue(breadcrumb, 'message');
  if (!args.length) return typeof message === 'string' ? redactMessage(message, redactions) : undefined;
  return args
    .map((item, index) => {
      if (typeof item === 'string') return index === 0 ? redactMessage(item, redactions) : undefined;
      if (item instanceof Error) {
        const message = ownValue(item, 'message');
        return typeof message === 'string' ? redactMessage(message, [...redactions, ...privateChain(item)]) : 'Error';
      }
      if (typeof item === 'number' || typeof item === 'boolean') return String(item);
      return undefined;
    })
    .filter((item) => item !== undefined)
    .join(' ');
}

/** sanitizeBreadcrumb preserves safe console, navigation, request and UI context while excluding private input. */
export function sanitizeBreadcrumb(breadcrumb, hint = {}, eventRedactions = []) {
  try {
    if (!breadcrumb || typeof breadcrumb !== 'object') return null;
    const clean = cloneData(breadcrumb, eventRedactions);
    const category = ownValue(breadcrumb, 'category');
    if (typeof category === 'string' && category.startsWith('ui.')) {
      const target = hint.event?.target ?? hint.target;
      if (typeof Element !== 'undefined' && target instanceof Element) {
        if (target.closest('input,textarea,pre,code,[contenteditable="true"],[data-sentry-private]')) return null;
        clean.message =
          target.tagName.toLowerCase() +
          [...target.classList]
            .slice(0, 3)
            .filter((name) => /^[a-z][a-z0-9:_-]{0,60}$/i.test(name))
            .map((name) => '.' + sanitizeText(name))
            .join('');
      } else if (typeof clean.message === 'string') {
        clean.message = clean.message.replace(/\[(?:aria-label|alt|title|value)="[^"]*"\]/g, '').replace(/#[^ .\[>]+/g, '');
      }
      if (clean.data)
        for (const key of ['value', 'text', 'label', 'id', 'title', 'aria_label', 'aria-label', 'innerText', 'textContent'])
          delete clean.data[key];
    }
    if (category === 'console') {
      const explicitArguments = ownValue(ownValue(breadcrumb, 'data'), 'arguments');
      const input = ownValue(hint, 'input');
      const args = Array.isArray(input) ? input : Array.isArray(explicitArguments) ? explicitArguments : [];
      const redactions = [...eventRedactions, ...arrayValues(args, 50).flatMap((item) => privateChain(item))];
      clean.message = consoleMessage(breadcrumb, args, redactions);
      if (clean.data) delete clean.data.arguments;
    }
    return clean;
  } catch {
    try {
      console.warn('Browser error breadcrumb filtering unavailable.');
    } catch {
      /* Preserve native UI behavior when console reporting fails. */
    }
    return null;
  }
}

/** sanitizeEvent preserves normal SDK diagnostics and applies authoritative immutable public deployment identifiers. */
export function sanitizeEvent(event, config, hint = {}) {
  try {
    hint.attachments = [];
    if (!event || typeof event !== 'object') return null;
    if (ownValue(event, 'type') !== undefined) return null;
    const originalException = ownValue(hint, 'originalException');
    const clean = cloneData(event, privateChain(originalException));
    if (!clean || typeof clean !== 'object') return null;
    delete clean.user;
    if (clean.request) {
      delete clean.request.headers;
      delete clean.request.cookies;
      delete clean.request.data;
      delete clean.request.query_string;
    }
    if (clean.extra) delete clean.extra.arguments;
    // Core diagnostics get a separate budget so wide scope metadata cannot exhaust a normal SDK stack.
    const core = { nodes: 16_384, entries: 32_768, characters: 131_072, seen: new WeakSet() };
    const redactions = privateChain(originalException);
    for (const key of ['message', 'logentry', 'exception', 'sdk', 'debug_meta']) {
      const value = ownValue(event, key);
      if (value !== undefined) clean[key] = cloneData(value, redactions, key, '', 0, core);
    }
    const breadcrumbs = ownValue(event, 'breadcrumbs');
    if (Array.isArray(breadcrumbs))
      clean.breadcrumbs = arrayValues(breadcrumbs, 200)
        .map((item) => sanitizeBreadcrumb(item, {}, redactions))
        .filter(Boolean);
    clean.environment = config.environment;
    clean.release = config.release;
    clean.tags = { ...clean.tags, site: config.site ?? 'one-api', theme: config.theme };
    return clean;
  } catch {
    try {
      console.warn('Browser error event filtering unavailable.');
    } catch {
      /* Preserve native UI behavior when console reporting fails. */
    }
    return null;
  }
}
