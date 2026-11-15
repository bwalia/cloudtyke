// hello-web: one page that proves the whole chain works.
// Each request: Kubernetes service-account token -> Entra token (workload
// identity) -> Key Vault secret -> Blob storage read/write.
// No dependencies, no secrets: everything comes from the environment the
// pod gets (AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_FEDERATED_TOKEN_FILE).
const http = require('http');
const fs = require('fs');
const os = require('os');

const domain = process.env.CLOUDTYKE_DOMAIN || 'cloudtyke.local';
const authority = process.env.AZURE_AUTHORITY_HOST || `https://login.${domain}`;
const { AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_FEDERATED_TOKEN_FILE, VAULT_URI, STORAGE_ACCOUNT } = process.env;
const blobUrl = `https://${STORAGE_ACCOUNT}.blob.storage.${domain}/visits/count.txt`;

async function token(scope, steps) {
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
  return b.access_token;
}

async function handle() {
  const steps = [];
  // 1. greeting from Key Vault
  const kvTok = await token(`https://vault.${domain}/.default`, steps);
  let t0 = Date.now();
  let r = await fetch(`${VAULT_URI.replace(/\/$/, '')}/secrets/greeting?api-version=7.5`, { headers: { Authorization: `Bearer ${kvTok}` } });
  const secret = await r.json();
  steps.push({ what: 'Key Vault: read secret "greeting"', status: r.status, ms: Date.now() - t0 });
  if (!r.ok) throw new Error(`Key Vault ${r.status}: ${secret.error?.message}`);

  // 2. visit counter in Blob storage
  const stTok = await token(`https://storage.${domain}/.default`, steps);
  const h = { Authorization: `Bearer ${stTok}`, 'x-ms-version': '2025-01-05' };
  t0 = Date.now();
  r = await fetch(blobUrl, { headers: h });
  const visits = (r.status === 404 ? 0 : Number(await r.text()) || 0) + 1;
  steps.push({ what: 'Blob: read visits/count.txt', status: r.status, ms: Date.now() - t0 });
  t0 = Date.now();
  r = await fetch(blobUrl, { method: 'PUT', body: String(visits), headers: { ...h, 'x-ms-blob-type': 'BlockBlob', 'Content-Type': 'text/plain' } });
  steps.push({ what: 'Blob: write visits/count.txt', status: r.status, ms: Date.now() - t0 });
  if (!r.ok) throw new Error(`Blob write ${r.status}: ${await r.text()}`);
  return { greeting: secret.value, visits, steps };
}

const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
const page = (body) => `<!doctype html><html><head><meta charset="utf-8"><title>hello-web on CloudTyke</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 20px;color:#1f2937}
h1{font-size:28px;margin-bottom:4px}.big{font-size:22px;background:#ecfdf5;border:1px solid #a7f3d0;padding:16px;border-radius:10px}
.err{background:#fef2f2;border-color:#fecaca}table{border-collapse:collapse;width:100%;margin-top:16px}td,th{text-align:left;padding:6px 8px;border-bottom:1px solid #e5e7eb;font-size:14px}
.ok{color:#15803d;font-weight:600}.no{color:#b91c1c;font-weight:600}.muted{color:#6b7280;font-size:13px}</style></head><body>${body}</body></html>`;

http.createServer(async (req, res) => {
  if (req.url === '/healthz') { res.end('ok'); return; }
  if (req.url !== '/') { res.writeHead(404); res.end(); return; }
  const where = `pod <b>${esc(os.hostname())}</b> on AKS cluster <b>${esc(process.env.CLUSTER_NAME || '?')}</b>`;
  try {
    const { greeting, visits, steps } = await handle();
    res.writeHead(200, { 'Content-Type': 'text/html' });
    res.end(page(`<h1>hello-web</h1><p class="muted">Served by ${where}</p>
<p class="big">${esc(greeting)}</p>
<p>This page has been viewed <b>${visits}</b> time${visits === 1 ? '' : 's'} (counter kept in Azure Blob storage).</p>
<h3>What happened for this request</h3><table><tr><th>Step</th><th>HTTP</th><th>ms</th></tr>
${steps.map((s) => `<tr><td>${esc(s.what)}</td><td class="${s.status < 300 || s.status === 404 ? 'ok' : 'no'}">${s.status}</td><td>${s.ms}</td></tr>`).join('')}</table>
<p class="muted">No secrets in this pod: it exchanged its Kubernetes service-account token for Entra tokens as
managed identity ${esc(AZURE_CLIENT_ID)}. Everything is emulated by CloudTyke.</p>`));
  } catch (err) {
    res.writeHead(500, { 'Content-Type': 'text/html' });
    res.end(page(`<h1>hello-web</h1><p class="muted">Served by ${where}</p><p class="big err">${esc(err.message)}</p>`));
  }
}).listen(8080, () => console.log('hello-web listening on :8080'));
