// Northwind Airways booking site (a fictional Contoso Pay merchant).
// The browser never sees card data leave for anywhere but this server, which
// calls the Contoso Pay gateway with the merchant API key from Key Vault.
const path = require('path');
const crypto = require('crypto');
const express = require('express');
const { trace, metrics } = require('@opentelemetry/api');
const azure = require('./lib/azure');
const cfg = require('./lib/config');
const log = require('./lib/log');

const meter = metrics.getMeter('northwind.storefront');
const mCheckouts = meter.createCounter('northwind.checkouts', { description: 'Northwind Airways checkout attempts' });
const mWebhooks = meter.createCounter('northwind.webhooks.received', { description: 'Contoso Pay webhooks received' });

const FLIGHTS = [
  { code: 'NW 101', from: 'London Heathrow (LHR)', to: 'New York JFK (JFK)', depart: '08:40', arrive: '11:35', fares: { Economy: 48900, Premium: 112000, Business: 324500 } },
  { code: 'NW 214', from: 'London Heathrow (LHR)', to: 'Dubai (DXB)', depart: '13:10', arrive: '23:55', fares: { Economy: 39900, Premium: 89000, Business: 251000 } },
  { code: 'NW 330', from: 'Manchester (MAN)', to: 'Barcelona (BCN)', depart: '06:55', arrive: '10:20', fares: { Economy: 8900, Premium: 15900, Business: 42000 } },
  { code: 'NW 452', from: 'London Gatwick (LGW)', to: 'Reykjavik (KEF)', depart: '10:05', arrive: '13:15', fares: { Economy: 12900, Premium: 23900, Business: 61000 } },
  { code: 'NW 508', from: 'Edinburgh (EDI)', to: 'Toronto (YYZ)', depart: '12:30', arrive: '15:05', fares: { Economy: 41500, Premium: 98000, Business: 289000 } },
  { code: 'NW 777', from: 'London Heathrow (LHR)', to: 'Singapore (SIN)', depart: '21:15', arrive: '18:05+1', fares: { Economy: 61900, Premium: 145000, Business: 412000 } },
];

const app = express();
app.use(express.json());
app.get('/healthz', (_req, res) => res.json({ ok: true }));
app.use(express.static(path.join(__dirname, '..', 'public', 'storefront')));
app.get('/api/flights', (_req, res) => res.json(FLIGHTS));

const apiKey = () => azure.getSecret(cfg.merchantVault(), 'northwind-airways-api-key');

app.post('/api/checkout', async (req, res, next) => {
  try {
    const { flight, cabin = 'Economy', passengers = 1, passenger = {}, card = {}, ipCountry } = req.body || {};
    const f = FLIGHTS.find((x) => x.code === flight);
    if (!f || !f.fares[cabin]) return res.status(400).json({ error: 'unknown_flight_or_cabin' });
    const pax = Math.max(1, Math.min(9, Number(passengers) || 1));
    const amount = f.fares[cabin] * pax + 2400 * pax; // fare + taxes, fees and charges
    const booking = 'NW' + crypto.randomBytes(3).toString('hex').toUpperCase();
    trace.getActiveSpan()?.setAttributes({ 'northwind.booking': booking, 'northwind.flight': f.code, 'northwind.cabin': cabin, 'northwind.passengers': pax });
    const r = await fetch(`${cfg.gatewayUrl()}/v1/payments`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${await apiKey()}`, 'Idempotency-Key': booking },
      body: JSON.stringify({ amount, currency: 'GBP', reference: `${booking} ${f.code} ${cabin}`, card, customer: { name: passenger.name, email: passenger.email, ip_country: ipCountry || 'GB' } }),
    });
    const payment = await r.json();
    const outcome = payment.status === 'declined' ? 'declined' : r.ok ? 'confirmed' : 'error';
    mCheckouts.add(1, { outcome, cabin, route: `${f.from.slice(-4, -1)}-${f.to.slice(-4, -1)}` });
    log.info({ booking, flight: f.code, outcome, payment: payment.id }, 'checkout');
    res.status(r.ok || r.status === 402 ? 200 : 502).json({ booking, flight: f, cabin, passengers: pax, amount, outcome, payment });
  } catch (err) { next(err); }
});

app.get('/api/receipt/:id', async (req, res, next) => {
  try {
    const r = await fetch(`${cfg.gatewayUrl()}/v1/payments/${encodeURIComponent(req.params.id)}/receipt`, { headers: { Authorization: `Bearer ${await apiKey()}` } });
    res.status(r.status).type('json').send(await r.text());
  } catch (err) { next(err); }
});

app.post('/webhooks/contoso-pay', (req, res) => {
  mWebhooks.add(1, { type: req.body?.type || 'unknown' });
  log.info({ type: req.body?.type, payment: req.body?.data?.id }, 'webhook received');
  res.status(204).end();
});

app.use((err, _req, res, _next) => {
  trace.getActiveSpan()?.recordException(err);
  log.error({ err: err.message }, 'storefront error');
  res.status(500).json({ error: 'internal_error' });
});

app.listen(cfg.port(), () => log.info({ port: cfg.port() }, 'Northwind Airways storefront listening'));
