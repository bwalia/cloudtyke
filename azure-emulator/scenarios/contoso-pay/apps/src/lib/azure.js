// Minimal Azure data-plane client over fetch: Entra tokens (client secret or
// workload identity), Key Vault secrets, Blob and Azure Files. Every call is an
// outgoing HTTP span thanks to the undici instrumentation.
const fs = require('fs');
const log = require('./log');

const domain = process.env.CLOUDTYKE_DOMAIN || 'cloudtyke.local';
const authority = (process.env.AZURE_AUTHORITY_HOST || `https://login.${domain}`).replace(/\/$/, '');
const tenant = process.env.AZURE_TENANT_ID;
const clientId = process.env.AZURE_CLIENT_ID;
const VAULT_SCOPE = `https://vault.${domain}/.default`;
const STORAGE_SCOPE = `https://storage.${domain}/.default`;
const STORAGE_VERSION = '2025-01-05';

const tokens = new Map();

async function token(scope) {
  const hit = tokens.get(scope);
  if (hit && hit.expires > Date.now() + 60_000) return hit.value;
  const form = new URLSearchParams({ grant_type: 'client_credentials', client_id: clientId, scope });
  if (process.env.AZURE_FEDERATED_TOKEN_FILE) {
    // AKS workload identity: exchange the projected service-account token.
    form.set('client_assertion_type', 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer');
    form.set('client_assertion', fs.readFileSync(process.env.AZURE_FEDERATED_TOKEN_FILE, 'utf8').trim());
  } else {
    form.set('client_secret', process.env.AZURE_CLIENT_SECRET || '');
  }
  const r = await fetch(`${authority}/${tenant}/oauth2/v2.0/token`, { method: 'POST', body: form });
  const body = await r.json();
  if (!r.ok) throw new Error(`token for ${scope}: ${r.status} ${body.error_description || body.error}`);
  tokens.set(scope, { value: body.access_token, expires: Date.now() + body.expires_in * 1000 });
  return body.access_token;
}

async function call(scope, url, init = {}, okStatuses = []) {
  const headers = { Authorization: `Bearer ${await token(scope)}`, ...(init.headers || {}) };
  const r = await fetch(url, { ...init, headers });
  if (!r.ok && !okStatuses.includes(r.status)) {
    const text = await r.text();
    throw new Error(`${init.method || 'GET'} ${url}: ${r.status} ${text.slice(0, 300)}`);
  }
  return r;
}

// ---------------------------------------------------------------- Key Vault
const secretCache = new Map();
async function getSecret(vaultUri, name, { maxAgeMs = 300_000 } = {}) {
  const key = vaultUri + name;
  const hit = secretCache.get(key);
  if (hit && hit.at > Date.now() - maxAgeMs) return hit.value;
  const r = await call(VAULT_SCOPE, `${vaultUri.replace(/\/$/, '')}/secrets/${name}?api-version=7.5`);
  const { value } = await r.json();
  secretCache.set(key, { value, at: Date.now() });
  return value;
}

// ---------------------------------------------------------------- Blob
const blobUrl = (account, container, name) => `https://${account}.blob.storage.${domain}/${container}/${name}`;
const sh = (extra = {}) => ({ 'x-ms-version': STORAGE_VERSION, ...extra });

async function putBlob(account, container, name, body, contentType = 'application/json') {
  await call(STORAGE_SCOPE, blobUrl(account, container, name), {
    method: 'PUT', body, headers: sh({ 'x-ms-blob-type': 'BlockBlob', 'Content-Type': contentType }),
  });
  return blobUrl(account, container, name);
}

async function getBlob(account, container, name) {
  const r = await call(STORAGE_SCOPE, blobUrl(account, container, name), { headers: sh() }, [404]);
  return r.status === 404 ? null : r.text();
}

// ---------------------------------------------------------------- Azure Files
const fileUrl = (account, share, path) => `https://${account}.file.storage.${domain}/${share}/${path}`;

async function ensureDirs(account, share, dir) {
  let acc = '';
  for (const part of dir.split('/').filter(Boolean)) {
    acc = acc ? `${acc}/${part}` : part;
    await call(STORAGE_SCOPE, `${fileUrl(account, share, acc)}?restype=directory`, {
      method: 'PUT', headers: sh({ 'x-ms-file-request-intent': 'backup' }),
    }, [409]);
  }
}

async function putFile(account, share, path, content) {
  const data = Buffer.from(content);
  const dir = path.split('/').slice(0, -1).join('/');
  if (dir) await ensureDirs(account, share, dir);
  const url = fileUrl(account, share, path);
  await call(STORAGE_SCOPE, url, { method: 'PUT', headers: sh({ 'x-ms-type': 'file', 'x-ms-content-length': String(data.length), 'x-ms-file-request-intent': 'backup' }) });
  if (data.length > 0) {
    await call(STORAGE_SCOPE, `${url}?comp=range`, {
      method: 'PUT', body: data,
      headers: sh({ 'x-ms-range': `bytes=0-${data.length - 1}`, 'x-ms-write': 'update', 'x-ms-file-request-intent': 'backup' }),
    });
  }
  return url;
}

async function getFile(account, share, path) {
  const r = await call(STORAGE_SCOPE, fileUrl(account, share, path), { headers: sh({ 'x-ms-file-request-intent': 'backup' }) }, [404]);
  return r.status === 404 ? null : r.text();
}

async function retry(what, fn, attempts = 60, delayMs = 3000) {
  for (let i = 1; ; i++) {
    try {
      return await fn();
    } catch (err) {
      if (i >= attempts) throw err;
      log.warn({ err: err.message, attempt: i }, `${what} not ready, retrying`);
      await new Promise((r) => setTimeout(r, delayMs));
    }
  }
}

module.exports = { token, getSecret, putBlob, getBlob, putFile, getFile, retry, domain };
