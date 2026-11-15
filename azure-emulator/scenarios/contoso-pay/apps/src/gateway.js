// Contoso Pay payment gateway (team: Payments Core).
//   POST /v1/payments                       authorise (and capture) a card payment
//   POST /v1/payments/:id/refunds           refund all or part of a captured payment
//   POST /v1/payments/:id/chargebacks       a card scheme raises a dispute (simulated)
//   GET  /v1/payments/:id[/receipt]         payment and its receipt (from Blob storage)
//   GET  /internal/*                        read models for the processor console
// Merchants authenticate with API keys held in the merchant Key Vault; card
// numbers are tokenised with a key from the payments Key Vault and never stored.
const crypto = require('crypto');
const express = require('express');
const { trace, metrics, SpanStatusCode } = require('@opentelemetry/api');
const azure = require('./lib/azure');
const cfg = require('./lib/config');
const stores = require('./lib/stores');
const log = require('./lib/log');

const tracer = trace.getTracer('contoso-pay.gateway');
const meter = metrics.getMeter('contoso-pay.gateway');
const mPayments = meter.createCounter('cpay.payments', { description: 'Payment attempts by outcome' });
const mVolume = meter.createCounter('cpay.payment.volume', { description: 'Authorised amount in minor units', unit: '{minor}' });
const mAcquirer = meter.createHistogram('cpay.acquirer.latency', { description: 'Acquirer authorisation latency', unit: 'ms' });
const mRefunds = meter.createCounter('cpay.refunds', { description: 'Refunds' });
const mChargebacks = meter.createCounter('cpay.chargebacks', { description: 'Chargebacks raised by card schemes' });
const mWebhooks = meter.createCounter('cpay.webhooks', { description: 'Webhook deliveries' });

const id = (p) => `${p}_${crypto.randomBytes(10).toString('base64url')}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---------------------------------------------------------------- schema (migrations run on start)
const SCHEMA = `
create table if not exists merchants (
  id text primary key, name text not null, mcc text not null, country text not null, currency text not null,
  created_at timestamptz not null default now());
create table if not exists payments (
  id text primary key, merchant_id text not null references merchants(id), reference text,
  amount_minor bigint not null, currency text not null, status text not null, decline_reason text,
  card_token text not null, card_brand text, card_last4 text, card_country text,
  customer_name text, customer_email text, ip_country text,
  risk_score int, risk_decision text, auth_code text, acquirer text, trace_id text,
  refunded_minor bigint not null default 0, settled_batch text,
  created_at timestamptz not null default now(), captured_at timestamptz);
create index if not exists payments_created on payments (created_at desc);
create index if not exists payments_merchant_status on payments (merchant_id, status);
create table if not exists refunds (
  id text primary key, payment_id text not null references payments(id), amount_minor bigint not null,
  reason text, created_at timestamptz not null default now());
create table if not exists chargebacks (
  id text primary key, payment_id text not null references payments(id), reason_code text not null,
  amount_minor bigint not null, status text not null, evidence_blob text, created_at timestamptz not null default now());
create table if not exists settlement_batches (
  id text primary key, merchant_id text not null references merchants(id), currency text not null,
  payment_count int not null, gross_minor bigint not null, refunds_minor bigint not null, fees_minor bigint not null,
  net_minor bigint not null, file_path text not null, created_at timestamptz not null default now());
`;

async function migrate() {
  const db = await stores.pg();
  await db.query(SCHEMA);
  for (const [mid, m] of Object.entries(cfg.merchants)) {
    await db.query('insert into merchants (id,name,mcc,country,currency) values ($1,$2,$3,$4,$5) on conflict (id) do nothing',
      [mid, m.name, m.mcc, m.country, m.currency]);
  }
}

// ---------------------------------------------------------------- merchant authentication
async function merchantFor(apiKey) {
  for (const mid of Object.keys(cfg.merchants)) {
    const key = await azure.getSecret(cfg.merchantVault(), `${mid}-api-key`);
    if (key && apiKey && crypto.timingSafeEqual(Buffer.from(sha(key)), Buffer.from(sha(apiKey)))) return mid;
  }
  return null;
}
const sha = (s) => crypto.createHash('sha256').update(s).digest('hex');

// ---------------------------------------------------------------- cards (public test numbers only)
const TEST_CARDS = {
  '4242424242424242': { brand: 'visa', country: 'GB' },
  '4000056655665556': { brand: 'visa', country: 'GB' },
  '5555555555554444': { brand: 'mastercard', country: 'GB' },
  '2223003122003222': { brand: 'mastercard', country: 'IE' },
  '378282246310005': { brand: 'amex', country: 'US' },
  '4000000000000002': { brand: 'visa', country: 'GB', decline: 'do_not_honour' },
  '4000000000009995': { brand: 'visa', country: 'GB', decline: 'insufficient_funds' },
  '4000000000000069': { brand: 'visa', country: 'GB', decline: 'expired_card' },
  '4000000000000127': { brand: 'visa', country: 'GB', decline: 'incorrect_cvc' },
  '4100000000000019': { brand: 'visa', country: 'NG' },
  '4000002760003184': { brand: 'visa', country: 'DE' },
};
const luhn = (n) => [...n].reverse().reduce((s, d, i) => s + (i % 2 ? [0, 2, 4, 6, 8, 1, 3, 5, 7, 9][+d] : +d), 0) % 10 === 0;

async function tokenise(pan) {
  const key = await azure.getSecret(cfg.paymentsVault(), 'pan-tokenisation-key');
  return 'tok_' + crypto.createHmac('sha256', key).update(pan).digest('base64url').slice(0, 24);
}

// ---------------------------------------------------------------- acquirer (simulated)
const ACQUIRERS = { GBP: 'Woodgrove Acquiring', EUR: 'Fabrikam Merchant Services', USD: 'Woodgrove Acquiring' };
async function acquirerAuthorise(payment, card) {
  const acquirer = ACQUIRERS[payment.currency] || 'Woodgrove Acquiring';
  return tracer.startActiveSpan('acquirer.authorise', { attributes: { 'cpay.acquirer': acquirer, 'card.brand': card.brand } }, async (span) => {
    await azure.getSecret(cfg.paymentsVault(), 'acquirer-api-key'); // credential for the acquirer API
    const started = Date.now();
    // Latency with a long tail, and the odd timeout from the issuer.
    const slow = Math.random() < 0.03;
    await sleep(slow ? 900 + Math.random() * 1200 : 40 + Math.random() * 180);
    let outcome = card.decline ? 'declined' : 'approved';
    let reason = card.decline || null;
    if (!card.decline && slow && Math.random() < 0.3) { outcome = 'declined'; reason = 'issuer_unavailable'; }
    const ms = Date.now() - started;
    mAcquirer.record(ms, { acquirer, outcome });
    span.setAttributes({ 'cpay.acquirer.outcome': outcome, 'cpay.decline_reason': reason || '' });
    span.end();
    return { acquirer, outcome, reason, authCode: outcome === 'approved' ? String(100000 + crypto.randomInt(899999)) : null };
  });
}

// ---------------------------------------------------------------- webhooks
async function deliverWebhook(merchantId, type, payment) {
  const db = await stores.events();
  const event = { id: id('evt'), type, created: new Date().toISOString(), data: { id: payment.id, status: payment.status, amount: payment.amount_minor, currency: payment.currency, reference: payment.reference } };
  const body = JSON.stringify(event);
  const key = await azure.getSecret(cfg.paymentsVault(), 'webhook-signing-key');
  const signature = crypto.createHmac('sha256', key).update(body).digest('hex');
  let status = 'delivered';
  let httpStatus = 200;
  if (cfg.merchants[merchantId].webhook === 'storefront') {
    try {
      const r = await fetch(`${cfg.storefrontUrl()}/webhooks/contoso-pay`, { method: 'POST', body, headers: { 'Content-Type': 'application/json', 'Cpay-Signature': signature } });
      httpStatus = r.status;
      status = r.ok ? 'delivered' : 'failed';
    } catch (err) {
      status = 'failed';
      httpStatus = 0;
    }
  } else if (Math.random() < 0.02) {
    status = 'failed'; httpStatus = 503; // merchant endpoint down: retried later
  }
  mWebhooks.add(1, { merchant: merchantId, status });
  await db.collection('webhook_deliveries').insertOne({ merchantId, eventId: event.id, type, paymentId: payment.id, status, httpStatus, attempts: 1, createdAt: new Date() });
}

// ---------------------------------------------------------------- receipts
function receiptPath(p) {
  const d = new Date(p.created_at || Date.now());
  return `${d.getUTCFullYear()}/${String(d.getUTCMonth() + 1).padStart(2, '0')}/${String(d.getUTCDate()).padStart(2, '0')}/${p.id}.json`;
}

async function writeReceipt(p, merchant) {
  const receipt = {
    receipt: p.id, merchant: merchant.name, merchant_category_code: merchant.mcc, reference: p.reference,
    amount: (p.amount_minor / 100).toFixed(2), currency: p.currency, card: `${p.card_brand} •••• ${p.card_last4}`,
    auth_code: p.auth_code, acquirer: p.acquirer, customer: p.customer_name, issued_at: new Date().toISOString(),
    issuer: 'Contoso Pay Ltd (fictional), 1 Example Street, London',
  };
  await azure.putBlob(cfg.receiptsAccount(), 'receipts', receiptPath(p), JSON.stringify(receipt, null, 2));
}

// ---------------------------------------------------------------- API
const app = express();
app.use(express.json());
app.get('/healthz', (_req, res) => res.json({ ok: true }));

async function authenticate(req, res, next) {
  try {
    const mid = await merchantFor((req.get('authorization') || '').replace(/^Bearer /, ''));
    if (!mid) return res.status(401).json({ error: 'invalid_api_key' });
    req.merchantId = mid;
    trace.getActiveSpan()?.setAttribute('cpay.merchant', mid);
    next();
  } catch (err) { next(err); }
}

app.post('/v1/payments', authenticate, async (req, res, next) => {
  const span = trace.getActiveSpan();
  try {
    const { amount, currency, reference, card = {}, customer = {}, capture = true } = req.body || {};
    const pan = String(card.number || '').replace(/\s/g, '');
    if (!Number.isInteger(amount) || amount <= 0) return res.status(400).json({ error: 'invalid_amount', message: 'amount must be a positive integer in minor units' });
    if (!/^[0-9]{13,19}$/.test(pan) || !luhn(pan)) return res.status(400).json({ error: 'invalid_card_number' });
    const known = TEST_CARDS[pan];
    if (!known) return res.status(400).json({ error: 'card_not_supported', message: 'This demo only accepts published test card numbers.' });
    const merchant = cfg.merchants[req.merchantId];
    const db = await stores.pg();
    const p = {
      id: id('pay'), merchant_id: req.merchantId, reference: reference || null, amount_minor: amount, currency: currency || merchant.currency,
      card_token: await tokenise(pan), card_brand: known.brand, card_last4: pan.slice(-4), card_country: known.country,
      customer_name: customer.name || null, customer_email: customer.email || null, ip_country: customer.ip_country || merchant.country,
      trace_id: span?.spanContext().traceId,
    };
    span?.setAttributes({ 'cpay.payment_id': p.id, 'cpay.amount_minor': amount, 'cpay.currency': p.currency, 'card.brand': known.brand });

    const riskRes = await fetch(`${cfg.riskUrl()}/v1/score`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ paymentId: p.id, merchantId: p.merchant_id, amountMinor: amount, currency: p.currency, cardToken: p.card_token, cardCountry: known.country, ipCountry: p.ip_country, mcc: merchant.mcc }),
    });
    const risk = await riskRes.json();
    p.risk_score = risk.score;
    p.risk_decision = risk.decision;

    if (risk.decision === 'block') {
      p.status = 'declined'; p.decline_reason = 'risk_blocked';
    } else {
      const auth = await acquirerAuthorise(p, known);
      p.acquirer = auth.acquirer;
      p.auth_code = auth.authCode;
      if (auth.outcome === 'approved') p.status = capture ? 'captured' : 'authorized';
      else { p.status = 'declined'; p.decline_reason = auth.reason; }
    }
    if (p.status === 'captured') p.captured_at = new Date();

    await db.query(`insert into payments (id,merchant_id,reference,amount_minor,currency,status,decline_reason,card_token,card_brand,card_last4,card_country,
        customer_name,customer_email,ip_country,risk_score,risk_decision,auth_code,acquirer,trace_id,captured_at)
      values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
    [p.id, p.merchant_id, p.reference, p.amount_minor, p.currency, p.status, p.decline_reason || null, p.card_token, p.card_brand, p.card_last4, p.card_country,
      p.customer_name, p.customer_email, p.ip_country, p.risk_score, p.risk_decision, p.auth_code || null, p.acquirer || null, p.trace_id, p.captured_at || null]);

    const events = await stores.events();
    await events.collection('payment_events').insertOne({ paymentId: p.id, merchantId: p.merchant_id, type: `payment.${p.status}`, amountMinor: amount, currency: p.currency, riskScore: p.risk_score, declineReason: p.decline_reason || null, traceId: p.trace_id, createdAt: new Date() });

    if (p.status !== 'declined') await writeReceipt(p, merchant);
    mPayments.add(1, { merchant: p.merchant_id, status: p.status, decline_reason: p.decline_reason || 'none', card_brand: p.card_brand, risk_decision: p.risk_decision });
    if (p.status !== 'declined') mVolume.add(amount, { merchant: p.merchant_id, currency: p.currency });
    span?.setAttributes({ 'cpay.status': p.status, 'cpay.risk_score': p.risk_score });
    log.info({ payment: p.id, merchant: p.merchant_id, status: p.status, decline_reason: p.decline_reason, amount, currency: p.currency, risk: p.risk_score }, 'payment processed');

    deliverWebhook(p.merchant_id, `payment.${p.status}`, p).catch((err) => log.warn({ err: err.message }, 'webhook failed'));
    res.status(p.status === 'declined' ? 402 : 201).json(publicView(p));
  } catch (err) { next(err); }
});

app.post('/v1/payments/:id/refunds', authenticate, async (req, res, next) => {
  try {
    const db = await stores.pg();
    const { rows: [p] } = await db.query('select * from payments where id=$1 and merchant_id=$2', [req.params.id, req.merchantId]);
    if (!p) return res.status(404).json({ error: 'payment_not_found' });
    if (!['captured', 'partially_refunded'].includes(p.status)) return res.status(409).json({ error: 'not_refundable', status: p.status });
    const remaining = Number(p.amount_minor) - Number(p.refunded_minor);
    const amount = req.body?.amount ?? remaining;
    if (!Number.isInteger(amount) || amount <= 0 || amount > remaining) return res.status(400).json({ error: 'invalid_amount', remaining });
    const rid = id('rfnd');
    const status = amount === remaining ? 'refunded' : 'partially_refunded';
    await db.query('begin');
    await db.query('insert into refunds (id,payment_id,amount_minor,reason) values ($1,$2,$3,$4)', [rid, p.id, amount, req.body?.reason || 'requested_by_customer']);
    await db.query('update payments set refunded_minor=refunded_minor+$1, status=$2 where id=$3', [amount, status, p.id]);
    await db.query('commit');
    const events = await stores.events();
    await events.collection('payment_events').insertOne({ paymentId: p.id, merchantId: p.merchant_id, type: 'payment.refunded', amountMinor: amount, currency: p.currency, createdAt: new Date() });
    mRefunds.add(1, { merchant: p.merchant_id, reason: req.body?.reason || 'requested_by_customer' });
    log.info({ payment: p.id, refund: rid, amount }, 'refund issued');
    deliverWebhook(p.merchant_id, 'payment.refunded', { ...p, status }).catch(() => {});
    res.status(201).json({ id: rid, payment: p.id, amount, status });
  } catch (err) { await (await stores.pg()).query('rollback').catch(() => {}); next(err); }
});

// Card schemes raise disputes; the traffic generator plays the scheme.
app.post('/v1/payments/:id/chargebacks', async (req, res, next) => {
  try {
    const db = await stores.pg();
    const { rows: [p] } = await db.query('select * from payments where id=$1', [req.params.id]);
    if (!p) return res.status(404).json({ error: 'payment_not_found' });
    const cid = id('cb');
    const reason = req.body?.reason_code || '10.4'; // Visa: other fraud, card-absent environment
    const evidence = `${p.merchant_id}/${cid}.json`;
    await azure.putBlob(cfg.receiptsAccount(), 'chargeback-evidence', evidence, JSON.stringify({
      chargeback: cid, payment: p.id, reason_code: reason, amount_minor: p.amount_minor, currency: p.currency,
      risk_score: p.risk_score, ip_country: p.ip_country, card_country: p.card_country, auth_code: p.auth_code,
      receipt: `receipts/${receiptPath(p)}`, collected_at: new Date().toISOString(),
    }, null, 2));
    await db.query('insert into chargebacks (id,payment_id,reason_code,amount_minor,status,evidence_blob) values ($1,$2,$3,$4,$5,$6)', [cid, p.id, reason, p.amount_minor, 'needs_response', evidence]);
    await db.query("update payments set status='chargeback' where id=$1", [p.id]);
    const events = await stores.events();
    await events.collection('payment_events').insertOne({ paymentId: p.id, merchantId: p.merchant_id, type: 'payment.chargeback', reasonCode: reason, createdAt: new Date() });
    mChargebacks.add(1, { merchant: p.merchant_id, reason_code: reason });
    log.warn({ payment: p.id, chargeback: cid, reason_code: reason }, 'chargeback received');
    res.status(201).json({ id: cid, payment: p.id, reason_code: reason, evidence });
  } catch (err) { next(err); }
});

app.get('/v1/payments/:id', authenticate, async (req, res, next) => {
  try {
    const { rows: [p] } = await (await stores.pg()).query('select * from payments where id=$1 and merchant_id=$2', [req.params.id, req.merchantId]);
    return p ? res.json(publicView(p)) : res.status(404).json({ error: 'payment_not_found' });
  } catch (err) { next(err); }
});

app.get('/v1/payments/:id/receipt', authenticate, async (req, res, next) => {
  try {
    const { rows: [p] } = await (await stores.pg()).query('select * from payments where id=$1 and merchant_id=$2', [req.params.id, req.merchantId]);
    if (!p) return res.status(404).json({ error: 'payment_not_found' });
    const body = await azure.getBlob(cfg.receiptsAccount(), 'receipts', receiptPath(p));
    return body ? res.type('json').send(body) : res.status(404).json({ error: 'receipt_not_found' });
  } catch (err) { next(err); }
});

// ---------------------------------------------------------------- read models for the processor console
app.get('/internal/feed', async (req, res, next) => {
  try {
    const { rows } = await (await stores.pg()).query(
      `select p.*, m.name as merchant_name from payments p join merchants m on m.id=p.merchant_id
       where ($1::timestamptz is null or p.created_at > $1) order by p.created_at desc limit 60`, [req.query.since || null]);
    res.json(rows);
  } catch (err) { next(err); }
});

app.get('/internal/summary', async (_req, res, next) => {
  try {
    const db = await stores.pg();
    const window = "created_at > now() - interval '15 minutes'";
    const [kpi, byMerchant, declines, batches, chargebacks, review] = await Promise.all([
      db.query(`select count(*)::int as total,
          count(*) filter (where status<>'declined')::int as approved,
          coalesce(sum(amount_minor) filter (where status<>'declined' and currency='GBP'),0)::bigint as volume_gbp,
          coalesce(round(avg(risk_score)),0)::int as avg_risk,
          count(*) filter (where decline_reason='risk_blocked')::int as risk_blocked
        from payments where ${window}`),
      db.query(`select m.name, p.currency, count(*)::int as n, coalesce(sum(p.amount_minor) filter (where p.status<>'declined'),0)::bigint as volume,
          round(100.0*count(*) filter (where p.status<>'declined')/greatest(count(*),1),1)::float as auth_rate
        from payments p join merchants m on m.id=p.merchant_id where p.${window} group by 1,2 order by 3 desc`),
      db.query(`select decline_reason as reason, count(*)::int as n from payments where status='declined' and ${window} group by 1 order by 2 desc`),
      db.query(`select b.*, m.name as merchant_name from settlement_batches b join merchants m on m.id=b.merchant_id order by b.created_at desc limit 12`),
      db.query(`select c.*, p.merchant_id, p.currency from chargebacks c join payments p on p.id=c.payment_id order by c.created_at desc limit 8`),
      db.query(`select id, merchant_id, amount_minor, currency, risk_score, ip_country, card_country, trace_id, created_at from payments where risk_decision='review' order by created_at desc limit 8`),
    ]);
    const refunds = await db.query(`select count(*)::int as n, coalesce(sum(amount_minor),0)::bigint as amount from refunds where ${window}`);
    res.json({ kpi: kpi.rows[0], byMerchant: byMerchant.rows, declines: declines.rows, batches: batches.rows, chargebacks: chargebacks.rows, review: review.rows, refunds: refunds.rows[0] });
  } catch (err) { next(err); }
});

app.get('/internal/settlement-file', async (req, res, next) => {
  try {
    const body = await azure.getFile(cfg.settlementAccount(), 'settlement-outbound', String(req.query.path || ''));
    return body === null ? res.status(404).send('not found') : res.type('text/csv').send(body);
  } catch (err) { next(err); }
});

app.use((err, _req, res, _next) => {
  const span = trace.getActiveSpan();
  span?.recordException(err);
  span?.setStatus({ code: SpanStatusCode.ERROR, message: err.message });
  log.error({ err: err.message }, 'request failed');
  res.status(500).json({ error: 'internal_error' });
});

function publicView(p) {
  return {
    id: p.id, status: p.status, amount: Number(p.amount_minor), currency: p.currency, reference: p.reference,
    decline_reason: p.decline_reason || null, card: { brand: p.card_brand, last4: p.card_last4, country: p.card_country },
    risk: { score: p.risk_score, decision: p.risk_decision }, auth_code: p.auth_code || null, acquirer: p.acquirer || null,
    refunded: Number(p.refunded_minor || 0), trace_id: p.trace_id,
  };
}

azure.retry('payments database', migrate).then(() => {
  app.listen(cfg.port(), () => log.info({ port: cfg.port() }, 'gateway listening'));
}).catch((err) => { log.fatal({ err: err.message }, 'gateway failed to start'); process.exit(1); });
