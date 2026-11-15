// Contoso Pay processor console (team: Merchant Experience / Operations):
// live payments, authorisation rates, declines, risk review, chargebacks and
// settlement batches. Every row links to its distributed trace in Jaeger.
const path = require('path');
const express = require('express');
const cfg = require('./lib/config');
const log = require('./lib/log');

const app = express();
app.get('/healthz', (_req, res) => res.json({ ok: true }));
app.use(express.static(path.join(__dirname, '..', 'public', 'portal')));
app.get('/api/config', (_req, res) => res.json({ jaeger: cfg.jaegerUiUrl(), grafana: cfg.grafanaUrl() }));

const proxy = (to) => async (req, res) => {
  try {
    const qs = new URLSearchParams(req.query).toString();
    const r = await fetch(`${cfg.gatewayUrl()}${to}${qs ? '?' + qs : ''}`);
    res.status(r.status).type(r.headers.get('content-type') || 'application/json').send(await r.text());
  } catch (err) {
    log.error({ err: err.message }, 'gateway unreachable');
    res.status(502).json({ error: 'gateway_unreachable' });
  }
};
app.get('/api/feed', proxy('/internal/feed'));
app.get('/api/summary', proxy('/internal/summary'));
app.get('/api/settlement-file', proxy('/internal/settlement-file'));

app.listen(cfg.port(), () => log.info({ port: cfg.port() }, 'processor console listening'));
