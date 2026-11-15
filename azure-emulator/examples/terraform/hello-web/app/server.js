// hello-web: one page that proves the whole chain works.
// Each request: Kubernetes service-account token -> Entra token (workload
// identity) -> Key Vault secret -> Blob storage read/write.
// No dependencies, no secrets: everything comes from the environment the
// pod gets (AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_FEDERATED_TOKEN_FILE).
const http = require('http');
const fs = require('fs');
const os = require('os');
const { render } = require('./page');

const domain = process.env.CLOUDTYKE_DOMAIN || 'cloudtyke.local';
const authority = process.env.AZURE_AUTHORITY_HOST || `https://login.${domain}`;
const { AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_FEDERATED_TOKEN_FILE, VAULT_URI, STORAGE_ACCOUNT } = process.env;
const blobUrl = `https://${STORAGE_ACCOUNT}.blob.storage.${domain}/visits/count.txt`;

const claims = (jwt) => { try { return JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url')); } catch { return undefined; } };
const iac = () => { try { return JSON.parse(fs.readFileSync('/etc/hello-web/iac.json', 'utf8')); } catch { return undefined; } };

async function token(scope, steps, tokens) {
  const t0 = Date.now();
  const form = new URLSearchParams({
    grant_type: 'client_credentials', client_id: AZURE_CLIENT_ID, scope,
    client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
    client_assertion: fs.readFileSync(AZURE_FEDERATED_TOKEN_FILE, 'utf8').trim(),
  });
  const r = await fetch(`${authority}/${AZURE_TENANT_ID}/oauth2/v2.0/token`, { method: 'POST', body: form });
  const b = await r.json();
  steps.push({ what: `Entra token for ${scope.replace('/.default', '')} (workload identity)`, status: r.status, ms: Date.now() - t0 });
  if (!r.ok) throw new Error(b.error_description || b.error);
  tokens.push(claims(b.access_token));
  return b.access_token;
}

async function handle(ctx) {
  const { steps, tokens } = ctx;
  ctx.sa = claims(fs.readFileSync(AZURE_FEDERATED_TOKEN_FILE, 'utf8').trim());
  // 1. greeting from Key Vault
  const kvTok = await token(`https://vault.${domain}/.default`, steps, tokens);
  let t0 = Date.now();
  let r = await fetch(`${VAULT_URI.replace(/\/$/, '')}/secrets/greeting?api-version=7.5`, { headers: { Authorization: `Bearer ${kvTok}` } });
  const secret = await r.json();
  steps.push({ what: 'Key Vault: read secret "greeting"', status: r.status, ms: Date.now() - t0 });
  if (!r.ok) throw new Error(`Key Vault ${r.status}: ${secret.error?.message}`);

  // 2. visit counter in Blob storage
  const stTok = await token(`https://storage.${domain}/.default`, steps, tokens);
  const h = { Authorization: `Bearer ${stTok}`, 'x-ms-version': '2025-01-05' };
  t0 = Date.now();
  r = await fetch(blobUrl, { headers: h });
  const visits = (r.status === 404 ? 0 : Number(await r.text()) || 0) + 1;
  steps.push({ what: 'Blob: read visits/count.txt', status: r.status, ms: Date.now() - t0 });
  t0 = Date.now();
  r = await fetch(blobUrl, { method: 'PUT', body: String(visits), headers: { ...h, 'x-ms-blob-type': 'BlockBlob', 'Content-Type': 'text/plain' } });
  steps.push({ what: 'Blob: write visits/count.txt', status: r.status, ms: Date.now() - t0 });
  if (!r.ok) throw new Error(`Blob write ${r.status}: ${await r.text()}`);
  Object.assign(ctx, { greeting: secret.value, visits });
}

const env = () => ({
  pod: os.hostname(), cluster: process.env.CLUSTER_NAME || '?', domain, vaultUri: VAULT_URI, storage: STORAGE_ACCOUNT, iac: iac(),
});

http.createServer(async (req, res) => {
  if (req.url === '/healthz') { res.end('ok'); return; }
  if (req.url !== '/') { res.writeHead(404); res.end(); return; }
  const ctx = { steps: [], tokens: [] };
  try {
    await handle(ctx);
  } catch (err) {
    ctx.error = err.message;
  }
  res.writeHead(ctx.error ? 500 : 200, { 'Content-Type': 'text/html; charset=utf-8' });
  res.end(render(ctx, env()));
}).listen(8080, () => console.log('hello-web listening on :8080'));
