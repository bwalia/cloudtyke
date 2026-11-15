// Contoso Pay risk engine (team: Risk & Fraud). Scores each payment from
// amount, geography, merchant category and card velocity, and keeps every
// signal in Cosmos DB (risk_signals, 90-day TTL) for the fraud analysts.
const express = require('express');
const { trace, metrics } = require('@opentelemetry/api');
const stores = require('./lib/stores');
const cfg = require('./lib/config');
const log = require('./lib/log');
const azure = require('./lib/azure');

const meter = metrics.getMeter('contoso-pay.risk');
const mScore = meter.createHistogram('cpay.risk.score', { description: 'Risk score per payment (0-100)', advice: { explicitBucketBoundaries: [10, 20, 30, 40, 50, 60, 70, 80, 90, 100] } });
const mDecisions = meter.createCounter('cpay.risk.decisions', { description: 'Risk decisions' });
const HIGH_RISK_COUNTRIES = new Set(['NG', 'RU', 'KP']);

const app = express();
app.use(express.json());
app.get('/healthz', (_req, res) => res.json({ ok: true }));

app.post('/v1/score', async (req, res, next) => {
  try {
    const s = req.body;
    const db = await stores.events();
    const signals = db.collection('risk_signals');
    const since = new Date(Date.now() - 10 * 60_000);
    // Velocity per card + customer fingerprint (a stolen card tried repeatedly by one buyer).
    const velocity = await signals.countDocuments({ fingerprint: s.fingerprint, createdAt: { $gt: since } });
    const reasons = [];
    let score = Math.floor(Math.random() * 10);
    const gbp = s.currency === 'GBP' ? s.amountMinor : s.amountMinor * 0.85;
    if (gbp > 150000) { score += 25; reasons.push('high_amount'); }
    if (s.cardCountry !== s.ipCountry) { score += 30; reasons.push('geo_mismatch'); }
    if (HIGH_RISK_COUNTRIES.has(s.cardCountry) || HIGH_RISK_COUNTRIES.has(s.ipCountry)) { score += 45; reasons.push('high_risk_country'); }
    if (velocity >= 3) { score += 20 + Math.min(velocity, 10) * 4; reasons.push(`velocity_${velocity}_in_10m`); }
    if (s.mcc === '4511' && gbp > 200000) { score += 5; reasons.push('airline_premium_fare'); }
    score = Math.min(score, 100);
    const decision = score >= 80 ? 'block' : score >= 55 ? 'review' : 'approve';
    trace.getActiveSpan()?.setAttributes({ 'cpay.risk_score': score, 'cpay.risk_decision': decision, 'cpay.velocity': velocity });
    mScore.record(score, { merchant: s.merchantId, decision });
    mDecisions.add(1, { merchant: s.merchantId, decision });
    await signals.insertOne({ paymentId: s.paymentId, merchantId: s.merchantId, cardToken: s.cardToken, fingerprint: s.fingerprint, score, decision, reasons, velocity, ipCountry: s.ipCountry, cardCountry: s.cardCountry, createdAt: new Date() });
    if (decision !== 'approve') log.warn({ payment: s.paymentId, score, decision, reasons }, 'payment flagged by risk engine');
    res.json({ score, decision, reasons });
  } catch (err) { next(err); }
});

app.use((err, _req, res, _next) => {
  trace.getActiveSpan()?.recordException(err);
  log.error({ err: err.message }, 'scoring failed');
  res.status(500).json({ error: 'internal_error' });
});

azure.retry('risk signals store', async () => {
  const db = await stores.events();
  await db.collection('risk_signals').createIndex({ fingerprint: 1, createdAt: -1 });
}).then(() => app.listen(cfg.port(), () => log.info({ port: cfg.port() }, 'risk engine listening')))
  .catch((err) => { log.fatal({ err: err.message }, 'risk engine failed to start'); process.exit(1); });
