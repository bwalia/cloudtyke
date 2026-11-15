// Renders the hello-web page: the result, the Entra ID workflow as a sequence
// diagram annotated with this request's live results, the decoded tokens, a
// "where it runs" diagram and what OpenTofu/Terraform built.
const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
const short = (s, n = 44) => (String(s).length > n ? String(s).slice(0, n - 1) + '…' : String(s));

const C = {
  k8s: '#326ce5', pod: '#15803d', entra: '#0078d4', kv: '#b45309', blob: '#0e7490', arm: '#6d28d9',
  ink: '#1f2937', muted: '#6b7280', line: '#cbd5e1', ok: '#15803d', bad: '#b91c1c', skip: '#9ca3af',
};

// ------------------------------------------------------------ sequence diagram
function sequence(steps, d) {
  const L = { k8s: 80, pod: 250, entra: 460, kv: 670, blob: 870 };
  const W = 960, H = 790;
  const s = (i) => steps[i];
  const badge = (x, y, st) => {
    const ok = st && (st.status < 300 || st.status === 404);
    const txt = st ? `${st.status} · ${st.ms} ms` : 'not reached';
    const col = st ? (ok ? C.ok : C.bad) : C.skip;
    const w = txt.length * 6.6 + 16;
    return `<g><rect x="${x - w / 2}" y="${y - 11}" width="${w}" height="20" rx="10" fill="${col}"/>
<text x="${x}" y="${y + 3}" text-anchor="middle" class="bt">${txt}</text></g>`;
  };
  const arrow = (from, to, y, label, sub, color, dashed) => {
    const x1 = L[from], x2 = L[to], dir = x2 > x1 ? 1 : -1, mx = (x1 + x2) / 2;
    return `<line x1="${x1 + dir * 6}" y1="${y}" x2="${x2 - dir * 8}" y2="${y}" stroke="${color}" stroke-width="2"
${dashed ? 'stroke-dasharray="6 4"' : ''} marker-end="url(#ah-${color.slice(1)})"/>
${label ? `<text x="${mx}" y="${y - (sub ? 22 : 8)}" text-anchor="middle" class="ml">${label}</text>` : ''}
${sub ? `<text x="${mx}" y="${y - 8}" text-anchor="middle" class="ms">${sub}</text>` : ''}`;
  };
  const note = (x, y, w, lines, color) => `<g><rect x="${x}" y="${y}" width="${w}" height="${lines.length * 15 + 12}" rx="6"
fill="#fff" stroke="${color}" stroke-dasharray="3 3"/>${lines.map((t, i) =>
    `<text x="${x + 8}" y="${y + 19 + i * 15}" class="nt">${t}</text>`).join('')}</g>`;
  const phase = (y, t) => `<line x1="8" y1="${y - 4}" x2="${W - 8}" y2="${y - 4}" stroke="#e2e8f0"/>
<rect x="10" y="${y - 1}" width="${t.length * 7.4 + 12}" height="18" rx="4" fill="#f8fafc"/><text x="16" y="${y + 12}" class="ph">${t}</text>`;
  const act = (x, y1, y2, color) => `<rect x="${x - 5}" y="${y1}" width="10" height="${y2 - y1}" fill="${color}" opacity=".18" stroke="${color}"/>`;
  const head = (k, title, subtitle, color) => `<g><rect x="${L[k] - 78}" y="14" width="156" height="52" rx="10" fill="${color}"/>
<text x="${L[k]}" y="36" text-anchor="middle" class="ht">${title}</text>
<text x="${L[k]}" y="54" text-anchor="middle" class="hs">${subtitle}</text></g>
<line x1="${L[k]}" y1="66" x2="${L[k]}" y2="${H - 16}" stroke="${color}" stroke-width="1.5" stroke-dasharray="2 5" opacity=".6"/>`;
  const markers = [C.k8s, C.pod, C.entra, C.kv, C.blob].map((c) => `<marker id="ah-${c.slice(1)}" viewBox="0 0 10 10" refX="9" refY="5"
markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="${c}"/></marker>`).join('');

  return `<svg viewBox="0 0 ${W} ${H}" role="img" aria-label="Entra ID workload identity sequence" class="diagram">
<defs>${markers}</defs>
<style>.ht{font:600 14px system-ui,sans-serif;fill:#fff}.hs{font:11px ui-monospace,monospace;fill:#fff;opacity:.9}
.ml{font:600 12.5px system-ui,sans-serif;fill:${C.ink}}.ms{font:11px ui-monospace,monospace;fill:${C.muted}}
.nt{font:11.5px system-ui,sans-serif;fill:${C.ink}}.bt{font:600 11px ui-monospace,monospace;fill:#fff}
.ph{font:600 11px system-ui,sans-serif;fill:${C.muted};letter-spacing:.06em}</style>
${head('k8s', 'Kubernetes', 'kubelet · OIDC issuer', C.k8s)}
${head('pod', 'hello-web pod', short(d.pod, 22), C.pod)}
${head('entra', 'Entra ID', `login.${d.domain}`, C.entra)}
${head('kv', 'Key Vault', short(d.vault, 22), C.kv)}
${head('blob', 'Blob Storage', short(d.storage, 22), C.blob)}

${phase(96, '0 · BEFORE ANY REQUEST')}
${arrow('k8s', 'pod', 162, 'service-account token', 'projected, renewed hourly', C.k8s, true)}
${note(300, 128, 350, ['sub  system:serviceaccount:default:hello-web', 'aud  api://AzureADTokenExchange  ·  no Azure secret anywhere'], C.k8s)}

${phase(184, '1 · TOKEN FOR KEY VAULT')}
${act(L.entra, 226, 306, C.entra)}
${arrow('pod', 'entra', 236, 'POST …/oauth2/v2.0/token', 'assertion = SA token, scope vault', C.pod)}
${note(476, 238, 186, ['verify signature with the', 'cluster issuer’s public keys;', 'match federated credential', '(issuer + subject + audience)'], C.entra)}
${arrow('entra', 'pod', 300, 'access token', 'aud vault · oid id-hello-web', C.entra, true)}
${badge(L.pod - 88, 300, s(0))}

${phase(344, '2 · READ THE GREETING')}
${act(L.kv, 380, 456, C.kv)}
${arrow('pod', 'kv', 390, 'GET /secrets/greeting', 'Authorization: Bearer &lt;vault token&gt;', C.pod)}
${note(686, 396, 196, ['Azure RBAC check:', 'Key Vault Secrets User', '→ secrets/getSecret/action ✓'], C.kv)}
${arrow('kv', 'pod', 450, 'secret value', '', C.kv, true)}
${badge(L.pod - 88, 450, s(1))}

${phase(494, '3 · TOKEN FOR STORAGE')}
${act(L.entra, 530, 584, C.entra)}
${arrow('pod', 'entra', 540, 'same exchange', 'assertion = SA token, scope storage', C.pod)}
${arrow('entra', 'pod', 578, 'access token (aud storage)', '', C.entra, true)}
${badge(L.pod - 88, 578, s(2))}

${phase(622, '4 · COUNT THE VISIT')}
${act(L.blob, 658, 770, C.blob)}
${arrow('pod', 'blob', 668, 'GET, then PUT visits/count.txt', 'Bearer &lt;storage token&gt; · shared keys are disabled', C.pod)}
${note(678, 680, 188, ['Azure RBAC check:', 'Storage Blob Data Contributor', '→ blobs/read, blobs/write ✓'], C.blob)}
${arrow('blob', 'pod', 764, 'count, then 201 Created', '', C.blob, true)}
${badge(L.pod - 88, 740, s(3))}${badge(L.pod - 88, 766, s(4))}
<text x="104" y="744" text-anchor="end" class="ms">read</text><text x="104" y="770" text-anchor="end" class="ms">write</text>
</svg>`;
}

// ------------------------------------------------------------ architecture diagram
function architecture(d) {
  const box = (x, y, w, h, title, sub, color, fill = '#fff') => `<g><rect x="${x}" y="${y}" width="${w}" height="${h}" rx="8"
fill="${fill}" stroke="${color}" stroke-width="1.5"/><text x="${x + 10}" y="${y + 19}" class="bt2" fill="${color}">${title}</text>
${sub ? `<text x="${x + 10}" y="${y + 35}" class="bs">${sub}</text>` : ''}</g>`;
  const frame = (x, y, w, h, title, color) => `<g><rect x="${x}" y="${y}" width="${w}" height="${h}" rx="12" fill="${color}"
fill-opacity=".05" stroke="${color}" stroke-width="1.5"/><text x="${x + 12}" y="${y + 20}" class="ft" fill="${color}">${title}</text></g>`;
  const path = (dpath, color, dashed, label, lx, ly, anchor = 'middle') => `<path d="${dpath}" fill="none" stroke="${color}" stroke-width="2"
${dashed ? 'stroke-dasharray="6 4"' : ''} marker-end="url(#aa-${color.slice(1)})"/>
${label ? `<text x="${lx}" y="${ly}" text-anchor="${anchor}" class="al">${label}</text>` : ''}`;
  const markers = [C.pod, C.entra, C.kv, C.blob, C.muted, C.arm, C.k8s].map((c) => `<marker id="aa-${c.slice(1)}" viewBox="0 0 10 10"
refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="${c}"/></marker>`).join('');
  return `<svg viewBox="0 0 960 470" role="img" aria-label="Where every piece runs" class="diagram">
<defs>${markers}</defs>
<style>.ft{font:600 13px system-ui,sans-serif}.bt2{font:600 12.5px system-ui,sans-serif}.bs{font:11px ui-monospace,monospace;fill:${C.muted}}
.al{font:11px system-ui,sans-serif;fill:${C.ink}}.lg{font:11.5px system-ui,sans-serif;fill:${C.ink}}</style>
${frame(150, 8, 802, 418, 'Your machine · Docker', C.muted)}

${box(8, 70, 124, 52, 'Browser', 'you, or the LAN', C.ink)}
${box(170, 70, 150, 52, 'hello-web-proxy', 'socat · or Caddy+auth', C.ink)}

${frame(346, 38, 290, 252, `k3s node = AKS ${short(d.cluster, 18)}`, C.k8s)}
${box(362, 70, 258, 44, 'Service hello-web', 'NodePort 30080 → port 8080', C.k8s)}
${box(362, 130, 258, 56, 'Pods hello-web ×2', 'node:22 · no secrets inside', C.pod, '#f0fdf4')}
${box(362, 202, 124, 72, 'kube-apiserver', 'signs SA tokens', C.k8s)}
${box(496, 202, 124, 72, 'CoreDNS', '*.cloudtyke.local →', C.k8s)}

${frame(662, 38, 280, 380, 'CloudTyke emulator container', C.entra)}
${box(678, 70, 248, 44, `Entra ID · login.${d.domain}`, 'issues tokens ① ③', C.entra)}
${box(678, 124, 248, 44, 'Key Vault · *.vault.', 'secret "greeting" ②', C.kv)}
${box(678, 178, 248, 44, 'Blob Storage · *.blob.storage.', 'visits/count.txt ④', C.blob)}
${box(678, 232, 248, 44, 'OIDC discovery · oidc.', 'keys Entra checks SA tokens with', C.entra)}
${box(678, 286, 248, 44, 'dns sidecar · 10.253.53.53', 'answers *.cloudtyke.local', C.muted)}
${box(678, 340, 248, 62, `ARM · management.${d.domain}`, 'AKS RP · Azure Policy · RBAC', C.arm)}
<text x="688" y="396" class="bs">role assignments, resource state</text>

${box(170, 340, 330, 72, 'toolbox container', `${short(d.iacVersion, 26)} · kubectl`, C.arm, '#faf5ff')}
<text x="180" y="402" class="bs">service principal ${short(d.deployer, 22)}</text>

${path('M132,96 L166,96', C.pod)}
${path('M320,96 L358,92', C.pod)}
${path('M490,114 L490,126', C.pod)}
${path('M620,138 L674,96', C.entra)}
${path('M620,158 L674,148', C.kv)}
${path('M620,178 L674,200', C.blob)}
${path('M620,252 C650,252 648,308 674,308', C.muted, true)}
${path('M500,378 L674,372', C.arm, true, `${d.tool} apply → ARM`, 590, 368)}
${path('M330,340 L420,278', C.k8s, true)}
<text x="316" y="306" class="al">kubectl (Entra token)</text>

<g transform="translate(150,444)"><line x1="0" y1="0" x2="28" y2="0" stroke="${C.pod}" stroke-width="2"/>
<text x="36" y="4" class="lg">every page view</text>
<line x1="150" y1="0" x2="178" y2="0" stroke="${C.arm}" stroke-width="2" stroke-dasharray="6 4"/>
<text x="186" y="4" class="lg">once, at deploy time (make hello-web)</text>
<line x1="430" y1="0" x2="458" y2="0" stroke="${C.muted}" stroke-width="2" stroke-dasharray="6 4"/>
<text x="466" y="4" class="lg">infrastructure (DNS, issuer keys)</text></g>
</svg>`;
}

// ------------------------------------------------------------ helpers for the token panels
const PURPOSE = {
  'random_string.suffix': 'a 5-letter suffix so global names (vault, storage) are unique',
  'azurerm_resource_group.hello': 'the folder for everything; tagged to satisfy the company baseline',
  'azurerm_policy_definition.no_public_blob': 'the guardrail rule: deny storage accounts with public blob access',
  'azurerm_resource_group_policy_assignment.no_public_blob': 'switches the rule on for this resource group',
  'azurerm_key_vault.hello': 'holds the greeting; RBAC mode, purge protection',
  'azurerm_role_assignment.me_kv': 'lets the deployer write the secret',
  'azurerm_key_vault_secret.greeting': 'the text at the top of this page (var.greeting)',
  'azurerm_storage_account.hello': 'Entra-only (shared keys off), private',
  'azurerm_storage_container.visits': 'where count.txt lives',
  'azurerm_kubernetes_cluster.hello': 'Entra-only cluster, Azure RBAC, OIDC issuer, workload identity',
  'azurerm_role_assignment.me_aks': 'lets the deployer run kubectl (Azure RBAC for Kubernetes)',
  'azurerm_user_assigned_identity.app': 'the app’s identity in Entra ID',
  'azurerm_federated_identity_credential.app': 'trust: this cluster’s service account may sign in as the identity',
  'azurerm_role_assignment.app_kv': 'the identity may read secrets in this vault',
  'azurerm_role_assignment.app_blob': 'the identity may read and write blobs in this account',
  'data.azurerm_client_config.current': 'who is running tofu (the deployer)',
};
const inPath = new Set(['azurerm_key_vault.hello', 'azurerm_key_vault_secret.greeting', 'azurerm_storage_account.hello',
  'azurerm_storage_container.visits', 'azurerm_kubernetes_cluster.hello', 'azurerm_user_assigned_identity.app',
  'azurerm_federated_identity_credential.app', 'azurerm_role_assignment.app_kv', 'azurerm_role_assignment.app_blob']);

function claimsTable(claims, keys, highlight = {}) {
  if (!claims) return '<p class="muted">not available</p>';
  const v = (k) => (Array.isArray(claims[k]) ? claims[k].join(', ') : typeof claims[k] === 'number' && /^(exp|iat|nbf)$/.test(k)
    ? `${claims[k]} (${new Date(claims[k] * 1000).toISOString().slice(11, 19)} UTC)` : claims[k]);
  return `<table class="claims">${keys.filter((k) => claims[k] !== undefined).map((k) =>
    `<tr><th>${k}</th><td${highlight[k] ? ` class="hl" title="${esc(highlight[k])}"` : ''}>${esc(v(k))}</td></tr>`).join('')}</table>`;
}

function render({ greeting, visits, steps, sa, tokens, error }, env) {
  const iac = env.iac || {};
  const res = iac.resources || [];
  const find = (addr) => res.find((r) => r.address === addr) || {};
  const fic = find('azurerm_federated_identity_credential.app');
  const uai = find('azurerm_user_assigned_identity.app');
  const me = find('data.azurerm_client_config.current');
  const kvTok = tokens[0];
  const d = {
    pod: env.pod, domain: env.domain, cluster: env.cluster,
    vault: (env.vaultUri || '').replace(/^https:\/\/|\/$/g, '').split('.')[0],
    storage: env.storage, tool: iac.tool || 'tofu', iacVersion: iac.version || env.iacTool || 'OpenTofu', deployer: me.client_id || 'ARM_CLIENT_ID',
  };
  const check = (label, a, b, ok = a !== undefined && b !== undefined && (Array.isArray(a) ? a.includes(b) : a === b)) =>
    `<li class="${ok ? 'pass' : a === undefined || b === undefined ? 'unk' : 'fail'}"><b>${ok ? '✓' : '?'}</b> ${label}
<code>${esc(short(Array.isArray(a) ? a.join(', ') : a ?? '—', 70))}</code></li>`;

  const result = error
    ? `<p class="big err">${esc(error)}</p>`
    : `<p class="big">${esc(greeting)}</p>
<p>This page has been viewed <b>${visits}</b> time${visits === 1 ? '' : 's'} (counter kept in Azure Blob storage).</p>`;

  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>hello-web on CloudTyke</title>
<style>
body{font:16px/1.55 system-ui,sans-serif;max-width:1000px;margin:32px auto;padding:0 20px;color:${C.ink}}
h1{font-size:30px;margin:0}h2{font-size:21px;margin:44px 0 6px;padding-top:12px;border-top:1px solid #e5e7eb}
h3{font-size:16px;margin:20px 0 6px}
.big{font-size:22px;background:#ecfdf5;border:1px solid #a7f3d0;padding:16px;border-radius:10px}
.err{background:#fef2f2;border-color:#fecaca}
table{border-collapse:collapse;width:100%}td,th{text-align:left;padding:6px 8px;border-bottom:1px solid #e5e7eb;font-size:14px;vertical-align:top}
.ok{color:${C.ok};font-weight:600}.no{color:${C.bad};font-weight:600}.muted{color:${C.muted};font-size:14px}
.diagram{width:100%;height:auto;background:#f8fafc;border:1px solid #e5e7eb;border-radius:12px;margin:10px 0}
nav a{margin-right:14px;font-size:14px}
.cols{display:grid;grid-template-columns:1fr 1fr;gap:18px}@media(max-width:760px){.cols{grid-template-columns:1fr}}
.card{border:1px solid #e5e7eb;border-radius:10px;padding:12px 14px;background:#fff}
.claims th{width:80px;font:12px ui-monospace,monospace;color:${C.muted}}.claims td{font:12px ui-monospace,monospace;word-break:break-all}
.hl{background:#fef9c3}
ul.checks{list-style:none;padding:0}ul.checks li{padding:4px 0;font-size:14px}ul.checks b{display:inline-block;width:20px}
.pass b{color:${C.ok}}.fail b{color:${C.bad}}.unk b{color:${C.skip}}code{font:12.5px ui-monospace,monospace;background:#f1f5f9;padding:1px 5px;border-radius:4px}
ol li{margin:6px 0}pre{background:#0f172a;color:#e2e8f0;padding:12px 14px;border-radius:10px;font-size:13px;overflow-x:auto}
tr.path td:first-child{border-left:3px solid ${C.pod}}
</style></head><body>
<h1>hello-web</h1>
<p class="muted">Served by pod <b>${esc(env.pod)}</b> on AKS cluster <b>${esc(env.cluster)}</b> · everything below is emulated by CloudTyke</p>
${result}
<nav><a href="#flow">Entra ID workflow</a><a href="#tokens">The tokens</a><a href="#where">Where it runs</a><a href="#iac">What ${esc(iac.tool || 'tofu')} built</a></nav>

<h2 id="flow">The Entra ID workflow for this request</h2>
<p>The pod has <b>no password, key or connection string</b>. It proves who it is with a token Kubernetes gave it, swaps that
for Entra ID tokens (<i>workload identity federation</i>), and uses them to call Key Vault and Blob storage. The badges are
this request's real results.</p>
${sequence(steps, d)}
<details><summary class="muted">This request as a table</summary><table><tr><th>Step</th><th>HTTP</th><th>ms</th></tr>
${steps.map((x) => `<tr><td>${esc(x.what)}</td><td class="${x.status < 300 || x.status === 404 ? 'ok' : 'no'}">${x.status}</td><td>${x.ms}</td></tr>`).join('\n')}</table></details>
<ol>
<li><b>Trust is set up once, at deploy time.</b> OpenTofu created the managed identity <code>${esc(uai.name || 'id-hello-web')}</code>
and a <i>federated credential</i> on it: "accept tokens from this cluster's OIDC issuer, for service account
<code>default/hello-web</code>, with audience <code>api://AzureADTokenExchange</code>". No secret was created.</li>
<li><b>Kubernetes gives the pod a signed token.</b> The kubelet writes a service-account token into the pod
(<code>/var/run/secrets/azure/tokens/</code>) and renews it hourly. It says <i>who</i> the pod is, signed by the cluster.</li>
<li><b>The pod asks Entra ID for a token.</b> An OAuth 2.0 <i>client credentials</i> request where the "password" is the
Kubernetes token (<code>client_assertion</code>), for one resource at a time (<code>scope=https://vault…/.default</code>).</li>
<li><b>Entra ID checks the trust.</b> It downloads the cluster issuer's public keys, verifies the signature and expiry, and finds
the federated credential whose issuer, subject and audience all match. Then it issues an access token for the identity:
<code>appid</code> = the identity's client ID, <code>oid</code> = its principal ID, <code>aud</code> = the resource.</li>
<li><b>Key Vault and Storage check the token, then Azure RBAC.</b> Valid signature, right <code>aud</code>, not expired; then
"does <code>oid</code> hold a role at this scope that allows this <i>data action</i>?" The roles come from OpenTofu:
<i>Key Vault Secrets User</i> and <i>Storage Blob Data Contributor</i>. Remove one and the badge turns red with a 403.</li>
</ol>
<p class="muted">This demo asks for fresh tokens on every request so you can watch the exchange; real apps cache them until
shortly before they expire (the Azure Identity SDKs do this for you).</p>

<h2 id="tokens">The tokens, decoded</h2>
<p>Tokens are JWTs: three base64 parts (header, claims, signature). These are the claims of this request's tokens; the tokens
themselves never leave the pod. Highlighted claims are the ones the checks below compare.</p>
<div class="cols">
<div class="card"><h3>① Kubernetes service-account token</h3><p class="muted">issued by the cluster, sent to Entra ID</p>
${claimsTable(sa, ['iss', 'sub', 'aud', 'iat', 'exp'], { iss: 'matches the federated credential issuer', sub: 'matches the federated credential subject', aud: 'matches the federated credential audience' })}</div>
<div class="card"><h3>② Entra ID access token (Key Vault)</h3><p class="muted">issued by Entra ID, sent to Key Vault</p>
${claimsTable(kvTok, ['iss', 'aud', 'appid', 'app_displayname', 'oid', 'tid', 'idtyp', 'exp'], { appid: 'the managed identity client ID', oid: 'the principal that role assignments name', aud: 'Key Vault only accepts its own audience' })}</div>
</div>
<h3>Checks, live</h3>
<ul class="checks">
${check('SA token issuer = federated credential issuer (the AKS OIDC issuer)', sa?.iss, fic.issuer)}
${check('SA token subject = federated credential subject', sa?.sub, fic.subject)}
${check('SA token audience includes the federated credential audience', sa?.aud, (fic.audience || [])[0])}
${check(`Entra token appid = client ID of ${esc(uai.name || 'the identity')}`, kvTok?.appid, uai.client_id)}
${check('Entra token oid = principal ID that the role assignments grant', kvTok?.oid, uai.principal_id)}
${check('Entra token audience = the Key Vault resource', kvTok?.aud, `https://vault.${env.domain}`)}
</ul>

<h3>Three different sign-ins</h3>
<table>
<tr><th>Who</th><th>Signs in as</th><th>Credential</th><th>Token for</th><th>Allowed by</th></tr>
<tr><td>${esc(iac.tool || 'tofu')} in the toolbox</td><td>service principal <code>${esc(short(me.client_id || '?', 14))}</code></td><td>client secret (env var in the toolbox)</td><td>ARM, <code>management.</code></td><td>its role on the subscription</td></tr>
<tr><td>kubectl in the toolbox</td><td>the same service principal</td><td>client secret</td><td>the AKS server app <code>6dae42f8-…</code></td><td><i>AKS RBAC Cluster Admin</i> (<code>me_aks</code>); the cluster has no local accounts</td></tr>
<tr><td>this app</td><td>managed identity <code>${esc(uai.name || 'id-hello-web')}</code></td><td>federated: the Kubernetes token, <b>no secret</b></td><td><code>vault.</code>, <code>storage.</code></td><td><i>Key Vault Secrets User</i>, <i>Storage Blob Data Contributor</i></td></tr>
<tr><td>your browser</td><td>not Entra ID</td><td>basic auth at the proxy (LAN mode only)</td><td>n/a</td><td>the proxy</td></tr>
</table>

<h2 id="where">Where everything runs</h2>
<p>Every box is a container on one machine. The browser only ever talks to the proxy; the pod talks to the "Azure" services by
their real-looking names, which the cluster's DNS forwards to CloudTyke.</p>
${architecture(d)}
<table>
<tr><th>Service</th><th>Real Azure</th><th>Here</th><th>Used for</th></tr>
<tr><td>Entra ID</td><td><code>login.microsoftonline.com</code></td><td><code>login.${esc(env.domain)}</code></td><td>token exchange (steps ①, ③)</td></tr>
<tr><td>Key Vault</td><td><code>*.vault.azure.net</code></td><td><code>*.vault.${esc(env.domain)}</code></td><td>the greeting (②)</td></tr>
<tr><td>Blob Storage</td><td><code>*.blob.core.windows.net</code></td><td><code>*.blob.storage.${esc(env.domain)}</code></td><td>the counter (④)</td></tr>
<tr><td>Azure Resource Manager</td><td><code>management.azure.com</code></td><td><code>management.${esc(env.domain)}</code></td><td>OpenTofu: create, change, delete; policy and RBAC</td></tr>
<tr><td>AKS</td><td>managed control plane + VM nodes</td><td>one k3s container per cluster</td><td>runs the pods; issues their tokens</td></tr>
</table>

<h2 id="iac">What ${esc(iac.tool || 'tofu')} built</h2>
${iac.version ? `<p><b>${esc(iac.version)}</b> with ${(iac.providers || []).map((p) => `<code>${esc(p)}</code>`).join(' ')}
· applied ${esc(iac.applied_at || '')} from <code>examples/terraform/hello-web/main.tf</code></p>` : '<p class="muted">Deploy with make hello-web to see the inventory.</p>'}
<pre>make hello-web          # runs, inside the toolbox container:
${esc(iac.tool || 'tofu')} init               # downloads the azurerm + random providers
${esc(iac.tool || 'tofu')} apply              # plans and creates everything below, talking to ARM
kubectl apply …         # service account, deployment, service (with an Entra token)</pre>
<table>
<tr><th>Terraform address</th><th>Azure name</th><th>Why it exists</th></tr>
${res.map((r) => `<tr class="${inPath.has(r.address) ? 'path' : ''}"><td><code>${esc(r.address)}</code></td>
<td>${esc(r.name || (r.role ? `${r.role} → ${r.scope}` : ''))}</td><td>${esc(PURPOSE[r.address] || '')}</td></tr>`).join('')}
</table>
<p class="muted">Green edge: used on every page view. Try <code>TF_VAR_greeting="Hi" make hello-web</code> (one resource changes),
or <code>TF_VAR_allow_public_blob=true make hello-web</code> (Azure Policy denies it).</p>
</body></html>`;
}

module.exports = { render };
