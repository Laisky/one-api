const assert = require('node:assert/strict');
const http = require('node:http');
const { createRequire } = require('node:module');
const path = require('node:path');
const { test } = require('node:test');

const theme = process.env.FRONTEND_THEME || path.basename(process.cwd());
assert.ok(['air', 'berry', 'modern'].includes(theme), 'Run from a frontend or set FRONTEND_THEME');
const themeRequire = createRequire(path.resolve(__dirname, '../../web', theme, 'package.json'));
const redirects = createRequire(themeRequire.resolve('axios'))('follow-redirects');
const axios = themeRequire('axios').default;

// listen starts a synthetic HTTP fixture bound exclusively to loopback.
function listen(server) {
  return new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
}

// capture returns only synthetic request details to the test client.
function capture(req, res) {
  let body = '';
  req.setEncoding('utf8');
  req.on('data', chunk => { body += chunk; });
  req.on('end', () => {
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({ headers: req.headers, method: req.method, body }));
  });
}

// withRedirect supplies same-origin or cross-port loopback endpoints and closes both.
async function withRedirect(crossOrigin, run) {
  const target = http.createServer(capture);
  const source = http.createServer((req, res) => {
    if (req.url === '/capture') return capture(req, res);
    const port = crossOrigin ? target.address().port : source.address().port;
    res.writeHead(307, { Location: 'http://127.0.0.1:' + port + '/capture' });
    res.end();
  });
  await listen(target);
  await listen(source);
  try {
    await run('http://127.0.0.1:' + source.address().port + '/redirect');
  } finally {
    await Promise.all([new Promise(resolve => source.close(resolve)), new Promise(resolve => target.close(resolve))]);
  }
}

// request follows one synthetic redirect through the actual installed dependency.
function request(url, headers, sensitiveHeaders = []) {
  return new Promise((resolve, reject) => {
    const req = redirects.http.get(url, { headers, sensitiveHeaders, maxRedirects: 2 }, res => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', chunk => { body += chunk; });
      res.on('error', reject);
      res.on('end', () => {
        try { assert.equal(res.statusCode, 200, 'Loopback endpoint must accept the request'); resolve(JSON.parse(body)); } catch (error) { reject(error); }
      });
    });
    req.setTimeout(3000, () => req.destroy(new Error('Loopback fixture timed out')));
    req.on('error', reject);
  });
}

for (const crossOrigin of [false, true]) {
  const boundary = crossOrigin ? 'cross-origin' : 'same-origin';
  test(theme + ': array credential headers respect ' + boundary + ' redirects', async () => {
    await withRedirect(crossOrigin, async url => {
      const result = await request(url, ['Host', new URL(url).host, 'Authorization', 'synthetic-auth', 'Cookie', 'synthetic=cookie',
        'Proxy-Authorization', 'synthetic-proxy', 'X-Control', 'benign']);
      for (const [name, value] of [['authorization', 'synthetic-auth'], ['cookie', 'synthetic=cookie'],
        ['proxy-authorization', 'synthetic-proxy']]) {
        assert.equal(result.headers[name], crossOrigin ? undefined : value, name);
      }
      assert.equal(result.headers['x-control'], 'benign');
    });
  });
  test(theme + ': declared metacharacter headers respect ' + boundary + ' redirects', async () => {
    await withRedirect(crossOrigin, async url => {
      const result = await request(url, { 'X-Api^Key': 'synthetic-key', 'X-Token|Secret': 'synthetic-token',
        'X-Control': 'benign' }, ['X-Api^Key', 'X-Token|Secret']);
      assert.equal(result.headers['x-api^key'], crossOrigin ? undefined : 'synthetic-key');
      assert.equal(result.headers['x-token|secret'], crossOrigin ? undefined : 'synthetic-token');
      assert.equal(result.headers['x-control'], 'benign');
    });
  });
}

for (const method of ['get', 'post']) {
  test(theme + ': Axios Node ' + method + ' retains redirect and body compatibility', async () => {
    await withRedirect(true, async url => {
      const response = await axios({ url, method, adapter: 'http', proxy: false, timeout: 3000,
        headers: { Authorization: 'synthetic-auth', 'X-Control': 'benign' },
        ...(method === 'post' ? { data: 'synthetic-body' } : {}) });
      assert.equal(response.status, 200);
      assert.equal(response.data.method, method.toUpperCase());
      assert.equal(response.data.body, method === 'post' ? 'synthetic-body' : '');
      assert.equal(response.data.headers.authorization, undefined);
      assert.equal(response.data.headers['x-control'], 'benign');
    });
  });
}
