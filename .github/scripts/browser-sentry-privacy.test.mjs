import assert from 'node:assert/strict';
import test from 'node:test';
import {
  isExpectedRequestError,
  markPrivateError,
  privateResponseError,
  sanitizeBreadcrumb,
  sanitizeEvent,
} from '../../web/legacy/browser-sentry-privacy.mjs';

const config = Object.freeze({
  dsn: 'https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456',
  site: 'one-api',
  theme: 'modern',
  environment: 'production',
  release: 'one-api-modern@sha256:' + 'a'.repeat(64),
});
const privateText = 'synthetic-private-fixture';

/** ordinaryEvent returns synthetic useful diagnostics without private application data. */
function ordinaryEvent() {
  const frames = [];
  for (let index = 0; index < 64; index++) {
    frames.push({
      filename:
        index % 3 === 0
          ? 'https://cdn.example.test/vendor/widget.js'
          : index % 3 === 1
            ? 'https://oneapi.laisky.com/assets/lazy-settings-Abcd1234.js'
            : 'webpack:///src/components/Settings.tsx',
      function: 'renderSettings' + index,
      lineno: index + 1,
      colno: 4,
      in_app: index % 3 !== 0,
      context_line: 'return createElement(Settings);',
      pre_context: ['const visible = true;'],
      post_context: ['updateLayout();'],
      vars: { visible: true },
    });
  }
  return {
    event_id: 'b'.repeat(32),
    timestamp: 1234,
    level: 'error',
    message: 'Settings layout could not be updated',
    logentry: { message: 'Layout update failed at %s', params: ['SettingsPanel'] },
    exception: {
      values: [
        {
          type: 'SettingsLayoutError',
          value: 'The requested layout widget is missing',
          mechanism: { type: 'auto.browser.global_handlers.onerror', handled: false, data: { phase: 'render' } },
          stacktrace: { frames },
        },
      ],
    },
    extra: { componentStack: '\n    at SettingsPanel\n    at Application', phase: 'render' },
    contexts: { browser: { name: 'SyntheticBrowser', version: '1.0' }, app: { build: 'fixture' } },
    sdk: { name: 'sentry.javascript.react', version: '11.7.0', integrations: ['GlobalHandlers', 'LinkedErrors'] },
    debug_meta: {
      images: [
        {
          type: 'sourcemap',
          code_file: 'https://oneapi.laisky.com/assets/lazy-settings-Abcd1234.js',
          debug_id: '11111111-2222-4333-8444-555555555555',
        },
      ],
    },
    tags: { route: 'settings', site: 'untrusted-site', theme: 'untrusted-theme' },
    environment: 'untrusted-environment',
    release: 'untrusted-release',
  };
}

/** assertPrivateAbsent verifies that a sanitized synthetic value contains no private fixture text. */
function assertPrivateAbsent(value) {
  assert.doesNotMatch(JSON.stringify(value), /synthetic-private-fixture/);
}

/** sanitizeFixture invokes the real event sanitizer with authoritative immutable configuration. */
function sanitizeFixture(event, hint = {}) {
  return sanitizeEvent(event, config, hint);
}

/** expectedDiagnosticFrame returns a frame copy without local variables or embedded source context. */
function expectedDiagnosticFrame(frame) {
  const clean = { ...frame };
  delete clean.vars;
  delete clean.context_line;
  delete clean.pre_context;
  delete clean.post_context;
  return clean;
}

// preservesOrdinaryDiagnostics checks that normal errors retain their diagnostic fields and complete stack.
test('ordinary errors preserve custom types, messages, mechanisms, complete stacks and SDK metadata', function preservesOrdinaryDiagnostics() {
  const input = ordinaryEvent();
  const snapshot = structuredClone(input);
  const clean = sanitizeFixture(input);
  assert.ok(clean);
  assert.equal(clean.message, input.message);
  assert.deepEqual(clean.logentry, input.logentry);
  assert.equal(clean.exception.values[0].type, 'SettingsLayoutError');
  assert.equal(clean.exception.values[0].value, input.exception.values[0].value);
  assert.deepEqual(clean.exception.values[0].mechanism, input.exception.values[0].mechanism);
  assert.deepEqual(clean.exception.values[0].stacktrace.frames, input.exception.values[0].stacktrace.frames.map(expectedDiagnosticFrame));
  assert.equal(clean.exception.values[0].stacktrace.frames.length, 64);
  assert.equal(clean.extra.componentStack, input.extra.componentStack);
  assert.deepEqual(clean.contexts, input.contexts);
  assert.deepEqual(clean.sdk, input.sdk);
  assert.deepEqual(clean.debug_meta, input.debug_meta);
  assert.deepEqual(input, snapshot);
});

// appliesDeploymentIdentity verifies that immutable deployment fields override arbitrary event fields without disclosing the DSN.
test('deployment identity is authoritative and public collector configuration is not attached', function appliesDeploymentIdentity() {
  const clean = sanitizeFixture(ordinaryEvent());
  assert.equal(clean.tags.site, config.site);
  assert.equal(clean.tags.theme, config.theme);
  assert.equal(clean.tags.route, 'settings');
  assert.equal(clean.environment, config.environment);
  assert.equal(clean.release, config.release);
  assert.ok(!JSON.stringify(clean).includes(config.dsn));
  assert.ok(!JSON.stringify(clean).includes('0123456789abcdef0123456789abcdef'));
  for (const type of ['transaction', 'spans', 'replay_event', 'session', 'log', 'profile']) {
    const typedEvent = ordinaryEvent();
    typedEvent.type = type;
    const hint = { attachments: [{ filename: 'private.txt', data: privateText }] };
    assert.equal(sanitizeFixture(typedEvent, hint), null, type);
    assert.ok(!hint.attachments || hint.attachments.length === 0, type);
  }
  const messageOnly = sanitizeFixture({ message: 'Ordinary message-only failure' });
  assert.ok(messageOnly);
  assert.equal(messageOnly.message, 'Ordinary message-only failure');
});

// filtersCredentialAliases checks nested credential and application payload names while retaining safe sibling diagnostics.
test('nested credential aliases and private payload fields are filtered without erasing safe siblings', function filtersCredentialAliases() {
  const names = [
    'Authorization',
    'Cookie',
    'Set-Cookie',
    'api_key',
    'apiKey',
    'apiKeys',
    'api_token',
    'access_token',
    'accessToken',
    'refresh_token',
    'refreshToken',
    'session_token',
    'sso_token',
    'token',
    'tokens',
    'jwt',
    'password',
    'secret',
    'localStorage',
    'account',
    'accountId',
    'account_id',
    'session',
    'session_id',
    'prompt',
    'prompts',
    'completion',
    'completions',
    'messages',
    'chat_history',
    'requestBody',
    'responseBody',
    'rawResponse',
    'tool_arguments',
    'tool_results',
    'inputs',
    'outputs',
    'mcp_inputs',
    'mcp_arguments',
    'mcp_results',
  ];
  for (const name of names) {
    const input = ordinaryEvent();
    input.extra.details = { safeCount: 7, nested: { [name]: privateText, safePhase: 'render' } };
    input.tags.identity = { [name]: privateText, operation: 'render', token_count: 42 };
    input.contexts.identity = { [name]: privateText, browser: 'SyntheticBrowser', token_count: 42 };
    input.contexts.graphql = {
      document: privateText,
      variables: { input: privateText },
      operationName: 'ListModels',
      browser: 'SyntheticBrowser',
      status: 200,
    };
    input.extra.technical = { variables: { phase: 'render', count: 7 } };
    const clean = sanitizeFixture(input);
    assert.ok(clean, name);
    assert.equal(clean.extra.details.safeCount, 7, name);
    assert.equal(clean.extra.details.nested.safePhase, 'render', name);
    assert.equal(clean.tags.identity.operation, 'render', name);
    assert.equal(clean.tags.identity.token_count, 42, name);
    assert.equal(clean.contexts.identity.browser, 'SyntheticBrowser', name);
    assert.equal(clean.contexts.identity.token_count, 42, name);
    assert.equal(clean.contexts.graphql.operationName, 'ListModels', name);
    assert.equal(clean.contexts.graphql.browser, 'SyntheticBrowser', name);
    assert.equal(clean.contexts.graphql.status, 200, name);
    assert.deepEqual(clean.extra.technical.variables, { phase: 'render', count: 7 }, name);
    assertPrivateAbsent(clean);
    assert.equal(input.extra.details.nested[name], privateText);
  }
});

// filtersRequestAndIdentity verifies private HTTP and user data are removed while safe request metadata remains useful.
test('request bodies, cookies, authentication, user identity and automatic arguments are filtered', function filtersRequestAndIdentity() {
  const input = ordinaryEvent();
  input.request = {
    url: 'https://user:synthetic-private-fixture@oneapi.laisky.com/api/options?token=synthetic-private-fixture#synthetic-private-fixture',
    method: 'POST',
    headers: { Authorization: 'Bearer ' + privateText, Cookie: privateText, Accept: 'application/json' },
    cookies: { auth: privateText },
    data: { value: privateText },
    body: privateText,
    query_string: 'token=' + privateText,
  };
  input.user = { id: privateText, username: privateText, email: privateText, ip_address: '203.0.113.10' };
  input.extra.arguments = [privateText, { nested: privateText }];
  input.contexts.response = { status_code: 400, body: privateText, data: privateText, headers: { 'Set-Cookie': privateText } };
  const clean = sanitizeFixture(input);
  assert.ok(clean);
  assertPrivateAbsent(clean);
  assert.ok(!JSON.stringify(clean).includes('203.0.113.10'));
  assert.equal(clean.request.method, 'POST');
  assert.equal(clean.request.url, 'https://oneapi.laisky.com/api/options');
  assert.equal(clean.contexts.response.status_code, 400);
});

// clearsAttachments checks that beforeSend cannot leave private scope attachments available for an SDK envelope.
test('event hints cannot retain private attachments', function clearsAttachments() {
  const hint = { attachments: [{ filename: 'private.txt', data: privateText, contentType: 'text/plain' }] };
  const clean = sanitizeFixture(ordinaryEvent(), hint);
  assert.ok(clean);
  assert.ok(!hint.attachments || hint.attachments.length === 0);
  assertPrivateAbsent(clean);
});

// stripsURLSecrets verifies valid URL diagnostics retain their useful paths without user information, query strings or fragments.
test('URLs retain useful paths across lazy, vendor, external and protocol-relative frames', function stripsURLSecrets() {
  const input = ordinaryEvent();
  const sourceURLs = [
    'https://user:synthetic-private-fixture@cdn.example.test/vendor/widget.js?prompt=synthetic-private-fixture#synthetic-private-fixture',
    '//user:synthetic-private-fixture@oneapi.laisky.com/assets/lazy.js?token=synthetic-private-fixture#synthetic-private-fixture',
    '/assets/vendor.js?api_key=synthetic-private-fixture#synthetic-private-fixture',
  ];
  input.exception.values[0].stacktrace.frames = sourceURLs.map(function urlFrame(filename, index) {
    // urlFrame creates a synthetic frame for each supplied URL and index.
    return { filename, function: 'loadChunk' + index, lineno: index + 1, colno: 2 };
  });
  input.debug_meta.images[0].code_file = sourceURLs[0];
  input.extra.documentationURL = 'https://docs.example.test/widget?token=synthetic-private-fixture#synthetic-private-fixture';
  const clean = sanitizeFixture(input);
  assert.ok(clean);
  assertPrivateAbsent(clean);
  const frames = clean.exception.values[0].stacktrace.frames;
  assert.equal(frames.length, sourceURLs.length);
  for (let index = 0; index < frames.length; index++) {
    assert.match(frames[index].filename, /(?:widget|lazy|vendor)\.js$/);
    assert.doesNotMatch(frames[index].filename, /[?#@]/);
    assert.equal(frames[index].function, 'loadChunk' + index);
    assert.equal(frames[index].lineno, index + 1);
  }
  assert.equal(clean.debug_meta.images[0].code_file, 'https://cdn.example.test/vendor/widget.js');
});

// removesOpaqueURLPayloads checks that data and blob source content cannot enter diagnostics through filename fields.
test('opaque data and blob URLs do not expose inline source or origin payloads', function removesOpaqueURLPayloads() {
  const input = ordinaryEvent();
  input.exception.values[0].stacktrace.frames.push(
    { filename: 'data:text/javascript,' + privateText, function: 'inlineHandler', lineno: 1 },
    { filename: 'blob:https://oneapi.laisky.com/' + privateText, function: 'blobHandler', lineno: 2 }
  );
  Object.assign(input.exception.values[0].stacktrace.frames[0], {
    vars: { token: privateText },
    context_line: privateText,
    pre_context: [privateText],
    post_context: [privateText],
  });
  const clean = sanitizeFixture(input);
  assert.ok(clean);
  assertPrivateAbsent(clean);
  assert.equal(clean.exception.values[0].stacktrace.frames[0].function, 'renderSettings0');
  for (const field of ['vars', 'context_line', 'pre_context', 'post_context']) {
    assert.equal(Object.hasOwn(clean.exception.values[0].stacktrace.frames[0], field), false);
  }
});

// preservesSafeBreadcrumbs checks useful console, navigation and HTTP diagnostics remain available after targeted filtering.
test('normal console, navigation and HTTP breadcrumbs retain useful messages and status', function preservesSafeBreadcrumbs() {
  const consoleBreadcrumb = {
    category: 'console',
    level: 'warning',
    message: 'Settings render retried',
    data: { logger: 'console' },
    timestamp: 1234,
  };
  assert.deepEqual(sanitizeBreadcrumb(consoleBreadcrumb), consoleBreadcrumb);
  const navigation = sanitizeBreadcrumb({
    category: 'navigation',
    data: { from: '/settings?token=' + privateText, to: 'https://oneapi.laisky.com/channels#' + privateText },
  });
  assert.ok(navigation);
  assert.match(navigation.data.from, /\/settings$/);
  assert.equal(navigation.data.to, 'https://oneapi.laisky.com/channels');
  assertPrivateAbsent(navigation);
  const request = sanitizeBreadcrumb({
    category: 'fetch',
    type: 'http',
    data: { method: 'GET', url: '/api/status?token=' + privateText, status_code: 503, responseBody: privateText },
  });
  assert.ok(request);
  assert.equal(request.data.method, 'GET');
  assert.equal(request.data.status_code, 503);
  assert.match(request.data.url, /\/api\/status$/);
  assertPrivateAbsent(request);
});

// filtersPrivateBreadcrumbs checks private console argument objects and DOM labels cannot leak through breadcrumb text or data.
test('console arguments and DOM input labels are filtered without dropping ordinary console messages', function filtersPrivateBreadcrumbs() {
  const consoleArguments = ['Provider request failed', privateText, { completion: privateText }];
  const consoleBreadcrumb = {
    category: 'console',
    message: 'Provider request failed ' + privateText + ' [object Object]',
    data: { arguments: consoleArguments, phase: 'stream' },
  };
  const clean = sanitizeBreadcrumb(consoleBreadcrumb, { input: consoleArguments });
  assert.ok(clean);
  assert.equal(clean.message, 'Provider request failed');
  assert.equal(clean.data.phase, 'stream');
  assertPrivateAbsent(clean);
  const fallbackArguments = ['Provider request failed', privateText, { completion: privateText }, undefined];
  let argumentGetterCalls = 0;
  Object.defineProperty(fallbackArguments, '3', {
    enumerable: true,
    get: function throwsPrivateConsoleArgumentGetter() {
      // throwsPrivateConsoleArgumentGetter detects accidental evaluation of a synthetic console argument accessor.
      argumentGetterCalls++;
      throw new Error(privateText);
    },
  });
  const fallbackBreadcrumb = { ...consoleBreadcrumb, data: { arguments: fallbackArguments, phase: 'stream' } };
  const fallbackClean = sanitizeBreadcrumb(fallbackBreadcrumb);
  assert.ok(fallbackClean);
  assert.equal(fallbackClean.message, 'Provider request failed');
  assertPrivateAbsent(fallbackClean);
  const eventWithFallbackBreadcrumb = ordinaryEvent();
  eventWithFallbackBreadcrumb.breadcrumbs = [fallbackBreadcrumb];
  const eventWithCleanBreadcrumb = sanitizeFixture(eventWithFallbackBreadcrumb);
  assert.ok(eventWithCleanBreadcrumb);
  assert.equal(eventWithCleanBreadcrumb.breadcrumbs[0].message, 'Provider request failed');
  assertPrivateAbsent(eventWithCleanBreadcrumb);
  assert.equal(argumentGetterCalls, 0);
  const parseError = new SyntaxError('Invalid provider SSE data: ' + privateText);
  markPrivateError(parseError, 'Provider SSE parsing failed');
  const parseArguments = ['Failed to parse SSE data:', parseError];
  const privateConsole = sanitizeBreadcrumb(
    { category: 'console', message: 'Failed to parse SSE data: ' + parseError.message, data: { arguments: parseArguments } },
    { input: parseArguments }
  );
  assert.ok(privateConsole);
  assert.match(privateConsole.message, /Failed to parse SSE data:/);
  assertPrivateAbsent(privateConsole);
  for (const category of ['ui.click', 'ui.input']) {
    const crumb = sanitizeBreadcrumb({
      category,
      message: 'button[aria-label="' + privateText + '"]',
      data: { value: privateText, label: privateText },
    });
    assertPrivateAbsent(crumb);
  }
});

// marksPrivateErrorWithoutMutation verifies source marking preserves the UI error object and replaces only its telemetry representation.
test('private error markers preserve UI message, stack and identity while reporting a safe summary', function marksPrivateErrorWithoutMutation() {
  const error = new Error('Provider returned ' + privateText);
  const message = error.message;
  const stack = error.stack;
  const marked = markPrivateError(error, 'Provider request failed');
  assert.equal(marked, error);
  assert.equal(error.message, message);
  assert.equal(error.stack, stack);
  assert.equal(Object.keys(error).length, 0);
  const input = ordinaryEvent();
  input.message = error.message;
  input.exception.values[0].value = error.message;
  const clean = sanitizeFixture(input, { originalException: error });
  assert.ok(clean);
  assert.equal(clean.exception.values[0].value, 'Provider request failed');
  assertPrivateAbsent(clean);
  assert.equal(error.message, message);
  assert.equal(error.stack, stack);
});

// filtersMarkedLinkedCopies verifies linked causes, SDK prefixes and truncated copies of private errors receive source-specific summaries.
test('linked private causes and prefixed or truncated SDK copies are sanitized without generic erasure', function filtersMarkedLinkedCopies() {
  const cause = new Error(privateText + ':' + 'x'.repeat(500));
  markPrivateError(cause, 'Provider stream failed');
  const root = new Error('Settings render failed', { cause });
  const input = ordinaryEvent();
  input.exception.values = [
    { type: 'Error', value: cause.message },
    { type: 'Error', value: 'UnhandledRejection: ' + cause.message },
    { type: 'Error', value: cause.message.slice(0, 250) + '...' },
    { type: 'SettingsLayoutError', value: root.message, mechanism: { type: 'generic', handled: true } },
  ];
  input.extra.details = { explanation: cause.message, safe: 'retry available' };
  const clean = sanitizeFixture(input, { originalException: root });
  assert.ok(clean);
  assert.equal(clean.exception.values.length, 4);
  for (const value of clean.exception.values.slice(0, 3)) {
    assert.ok(value.value.includes('Provider stream failed'));
  }
  assert.equal(clean.exception.values[3].value, root.message);
  assert.equal(clean.exception.values[3].type, 'SettingsLayoutError');
  assert.equal(clean.extra.details.safe, 'retry available');
  assertPrivateAbsent(clean);
  const ordinaryMember = new Error('Ordinary widget loading failed');
  const aggregateMembers = [cause, undefined, ordinaryMember];
  let memberGetterCalls = 0;
  Object.defineProperty(aggregateMembers, '1', {
    enumerable: true,
    get: function throwsPrivateAggregateMemberGetter() {
      // throwsPrivateAggregateMemberGetter detects accidental evaluation of a synthetic aggregate member accessor.
      memberGetterCalls++;
      throw new Error(privateText);
    },
  });
  const aggregate = new AggregateError([], 'ordinary aggregate');
  aggregate.errors = aggregateMembers;
  const aggregateEvent = ordinaryEvent();
  aggregateEvent.exception.values = [
    { type: 'Error', value: cause.message },
    { type: 'Error', value: ordinaryMember.message },
    { type: 'AggregateError', value: aggregate.message },
  ];
  const cleanAggregate = sanitizeFixture(aggregateEvent, { originalException: aggregate });
  assert.ok(cleanAggregate);
  assert.equal(cleanAggregate.exception.values[0].value, 'Provider stream failed');
  assert.equal(cleanAggregate.exception.values[1].value, ordinaryMember.message);
  assert.equal(cleanAggregate.exception.values[2].value, 'ordinary aggregate');
  assert.equal(cleanAggregate.exception.values[2].type, 'AggregateError');
  assertPrivateAbsent(cleanAggregate);
  assert.equal(memberGetterCalls, 0);
  assert.equal(aggregate.errors, aggregateMembers);
});

// identifiesExpectedPrivateRequests verifies only explicitly expected request markers suppress request telemetry.
test('private response errors preserve UI text and expected-request classification is explicit', function identifiesExpectedPrivateRequests() {
  const error = privateResponseError(privateText, 'Provider response failed');
  assert.ok(error instanceof Error);
  assert.equal(error.message, privateText);
  assert.equal(isExpectedRequestError(error), true);
  const expected = new Error(privateText);
  markPrivateError(expected, 'Authentication required', true);
  assert.equal(isExpectedRequestError(expected), true);
  assert.equal(isExpectedRequestError(new Error('Wrapper failed', { cause: expected })), true);
  assert.equal(isExpectedRequestError(new AggregateError([error], 'ordinary aggregate')), false);
  assert.equal(isExpectedRequestError(new Error('Authentication required')), false);
  assert.equal(isExpectedRequestError(null), false);
  const input = ordinaryEvent();
  input.exception.values[0].value = error.message;
  const clean = sanitizeFixture(input, { originalException: error });
  assert.equal(clean.exception.values[0].value, 'Provider response failed');
  assertPrivateAbsent(clean);
});

// handlesHostileMetadata verifies cycles and throwing getters are skipped safely without mutation or unbounded output.
test('cycles, getters and deep or wide metadata stay bounded and do not mutate input', function handlesHostileMetadata() {
  const input = ordinaryEvent();
  const cyclic = { phase: 'render' };
  cyclic.self = cyclic;
  let getterCalls = 0;
  Object.defineProperty(cyclic, 'privateGetter', {
    enumerable: true,
    get: function throwsPrivateGetter() {
      // throwsPrivateGetter detects accidental execution of a synthetic hostile metadata accessor.
      getterCalls++;
      throw new Error(privateText);
    },
  });
  const deep = { phase: 'render' };
  let cursor = deep;
  for (let index = 0; index < 100; index++) {
    cursor.next = { safeIndex: index };
    cursor = cursor.next;
  }
  cursor.apiToken = privateText;
  const wide = {};
  for (let index = 0; index < 2000; index++) wide['safeField' + index] = 's'.repeat(2000);
  wide.access_token = privateText;
  input.extra.hostile = cyclic;
  input.extra.deep = deep;
  input.extra.serializable = {
    bigint: BigInt(7),
    symbol: Symbol(privateText),
    callback: function unsupportedMetadataCallback() {
      // unsupportedMetadataCallback supplies a function value that must be omitted from serialized telemetry.
      return privateText;
    },
    safe: 'metadata preserved',
    values: [BigInt(7), Symbol(privateText), 'safe array marker'],
  };
  input.extra.wide = wide;
  let clean;
  assert.doesNotThrow(function sanitizeHostileFixture() {
    // sanitizeHostileFixture invokes the sanitizer against synthetic hostile metadata and records its return value.
    clean = sanitizeFixture(input);
  });
  assert.ok(clean);
  assert.equal(clean.exception.values[0].value, input.exception.values[0].value);
  assert.equal(getterCalls, 0);
  for (const key of ['bigint', 'symbol', 'callback']) {
    assert.equal(Object.hasOwn(clean.extra.serializable, key), false, key);
  }
  assert.equal(clean.extra.serializable.safe, 'metadata preserved');
  assert.ok(clean.extra.serializable.values.includes('safe array marker'));
  assert.doesNotThrow(function serializeBoundedFixture() {
    // serializeBoundedFixture verifies returned metadata is JSON serializable without cycles.
    JSON.stringify(clean);
  });
  assert.ok(JSON.stringify(clean).length < 512000);
  assertPrivateAbsent(clean);
  assert.equal(input.extra.hostile.self, cyclic);
  assert.equal(cursor.apiToken, privateText);
  assert.equal(Object.keys(wide).length, 2001);
});

// filtersURLPathCredentials proves URL-specific fields use credential redaction without losing ordinary source correlation.
test('recognized credentials in URL paths are filtered while ordinary frames and Debug IDs remain intact', function filtersURLPathCredentials() {
  const key = 'sk-abcdefabcdefabcdef';
  const input = ordinaryEvent();
  const ordinaryPath = 'https://oneapi.laisky.com/assets/lazy-settings-Abcd1234.js';
  const debugId = input.debug_meta.images[0].debug_id;
  input.request = { url: 'https://oneapi.laisky.com/api/tokens/' + key + '?secret=fixture#private' };
  input.exception.values[0].stacktrace.frames[0].filename = '/assets/' + key + '.js';
  input.exception.values[0].stacktrace.frames[1].filename = ordinaryPath;
  input.extra.url = '//cdn.example.test/assets/' + key + '.js';
  input.debug_meta.images.push({
    type: 'sourcemap',
    code_file: 'https://cdn.example.test/assets/' + key + '.js',
    debug_id: '22222222-3333-4444-8555-666666666666',
  });
  input.message = 'Network failure at https://oneapi.laisky.com/api/tokens/' + key;
  const clean = sanitizeFixture(input);
  assert.ok(clean);
  assert.ok(!JSON.stringify(clean).includes(key));
  assert.match(clean.request.url, /\/api\/tokens\/\[Filtered\]$/);
  assert.equal(clean.exception.values[0].stacktrace.frames[0].filename, '/assets/[Filtered].js');
  assert.equal(clean.exception.values[0].stacktrace.frames[1].filename, ordinaryPath);
  assert.equal(clean.extra.url, '//cdn.example.test/assets/[Filtered].js');
  assert.equal(clean.debug_meta.images[0].code_file, ordinaryPath);
  assert.equal(clean.debug_meta.images[0].debug_id, debugId);
  assert.equal(clean.debug_meta.images[1].debug_id, input.debug_meta.images[1].debug_id);
  assert.match(clean.message, /^Network failure at /);
  assert.ok(input.request.url.includes(key));
});
