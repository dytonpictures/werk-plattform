// passende dokumentation fehlt noch
import { createServer, request as createHTTPProxyRequest } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, isAbsolute, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const publicRoot = fileURLToPath(new URL('./public/', import.meta.url));
const contentTypes = {
  '.css': 'text/css; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
  '.ico': 'image/x-icon',
  '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
};
const hopByHopHeaders = new Set([
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
]);

const listenAddress = parseListenAddress(
  process.env.WERK_DASHBOARD_ADDRESS ?? '127.0.0.1:3000',
);
const apiTarget = parseAPITarget(process.env.WERK_API_URL);
const pageRoutes = new Map([
  ['/', 'index.html'],
  ['/change-password', 'change-password.html'],
  ['/mfa', 'mfa.html'],
  ['/mfa-setup', 'mfa-setup.html'],
  ['/admin', 'admin.html'],
  ['/app', 'work.html'],
  ['/documents', 'documents.html'],
  ['/profile', 'profile.html'],
]);

function parseListenAddress(value) {
  const separator = value.lastIndexOf(':');
  if (separator < 0) {
    throw new Error('WERK_DASHBOARD_ADDRESS must be a host:port address');
  }

  let host = value.slice(0, separator) || '0.0.0.0';
  if (host.startsWith('[') && host.endsWith(']')) {
    host = host.slice(1, -1);
  }
  const port = Number(value.slice(separator + 1));
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error('WERK_DASHBOARD_ADDRESS contains an invalid port');
  }
  return { host, port };
}

function parseAPITarget(value) {
  if (!value?.trim()) {
    return null;
  }
  const target = new URL(value);
  if (target.protocol !== 'http:') {
    throw new Error('WERK_API_URL must use http for the local development proxy');
  }
  target.hash = '';
  target.search = '';
  return target;
}

function log(level, message, fields = {}) {
  const line = JSON.stringify({
    time: new Date().toISOString(),
    level,
    service: 'werk-dashboard',
    message,
    ...fields,
  });
  if (level === 'error') {
    console.error(line);
    return;
  }
  console.log(line);
}

function isAPIPath(pathname) {
  return (
    pathname === '/meta' ||
    ['/api', '/admin', '/service', '/health'].some(
      (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`),
    )
  );
}

function withoutHopByHopHeaders(headers) {
  return Object.fromEntries(
    Object.entries(headers).filter(
      ([name, value]) => value !== undefined && !hopByHopHeaders.has(name.toLowerCase()),
    ),
  );
}

function proxyToAPI(request, response, incomingURL) {
  const target = new URL(apiTarget);
  target.pathname = incomingURL.pathname;
  target.search = incomingURL.search;

  const headers = withoutHopByHopHeaders(request.headers);
  headers.host = target.host;
  headers['x-forwarded-host'] = request.headers.host ?? '';
  headers['x-forwarded-proto'] = 'http';
  headers['x-forwarded-for'] = request.socket.remoteAddress ?? '';

  const upstreamRequest = createHTTPProxyRequest(
    target,
    { method: request.method, headers },
    (upstreamResponse) => {
      response.writeHead(
        upstreamResponse.statusCode ?? 502,
        withoutHopByHopHeaders(upstreamResponse.headers),
      );
      upstreamResponse.pipe(response);
    },
  );

  upstreamRequest.on('error', (error) => {
    log('error', 'api proxy request failed', {
      method: request.method,
      path: incomingURL.pathname,
      error: error.message,
    });
    if (!response.headersSent) {
      response.writeHead(502, { 'content-type': 'application/json; charset=utf-8' });
    }
    if (!response.writableEnded) {
      response.end('{"status":"api-unavailable"}\n');
    }
  });
  request.on('aborted', () => upstreamRequest.destroy());
  request.pipe(upstreamRequest);
}

async function handleRequest(request, response) {
  response.setHeader('cache-control', 'no-store');
  response.setHeader('content-security-policy', "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'");
  response.setHeader('cross-origin-opener-policy', 'same-origin');
  response.setHeader('cross-origin-resource-policy', 'same-origin');
  response.setHeader('permissions-policy', 'camera=(), geolocation=(), microphone=()');
  response.setHeader('referrer-policy', 'no-referrer');
  response.setHeader('x-content-type-options', 'nosniff');
  response.setHeader('x-frame-options', 'DENY');

  const url = new URL(request.url ?? '/', 'http://dashboard.local');
  if (apiTarget && isAPIPath(url.pathname)) {
    proxyToAPI(request, response, url);
    return;
  }

  if (url.pathname === '/health/live') {
    response.writeHead(200, { 'content-type': 'application/json; charset=utf-8' });
    response.end('{"status":"ok"}\n');
    return;
  }

  if (request.method !== 'GET' && request.method !== 'HEAD') {
    response.writeHead(405, { allow: 'GET, HEAD' }).end();
    return;
  }

  try {
    const requestedPath = decodeURIComponent(url.pathname);
    const requested = pageRoutes.get(requestedPath) ?? requestedPath.slice(1);
    const filename = resolve(publicRoot, requested);
    const relativePath = relative(publicRoot, filename);
    if (relativePath.startsWith('..') || isAbsolute(relativePath)) {
      response.writeHead(404).end();
      return;
    }

    const data = await readFile(filename);
    const headers = {
      'content-length': data.length,
      'content-type': contentTypes[extname(filename)] ?? 'application/octet-stream',
    };
    response.writeHead(200, headers);
    response.end(request.method === 'HEAD' ? undefined : data);
  } catch {
    response.writeHead(404).end();
  }
}

const server = createServer((request, response) => {
  handleRequest(request, response).catch((error) => {
    log('error', 'dashboard request failed', { error: error.message });
    if (!response.headersSent) {
      response.writeHead(500, { 'content-type': 'application/json; charset=utf-8' });
    }
    if (!response.writableEnded) {
      response.end('{"status":"error"}\n');
    }
  });
});

server.on('error', (error) => {
  log('error', 'dashboard server failed', { error: error.message });
  process.exitCode = 1;
});

server.listen(listenAddress.port, listenAddress.host, () => {
  log('info', 'dashboard server started', {
    address: `${listenAddress.host}:${listenAddress.port}`,
    api_proxy: apiTarget?.origin ?? null,
  });
});

function shutdown(signal) {
  log('info', 'dashboard server stopping', { signal });
  const timeout = setTimeout(() => {
    log('error', 'dashboard graceful shutdown timed out');
    process.exit(1);
  }, 5000);
  timeout.unref();

  server.close((error) => {
    clearTimeout(timeout);
    if (error) {
      log('error', 'dashboard graceful shutdown failed', { error: error.message });
      process.exit(1);
    }
    log('info', 'dashboard server stopped');
    process.exit(0);
  });
}

process.once('SIGINT', () => shutdown('SIGINT'));
process.once('SIGTERM', () => shutdown('SIGTERM'));
