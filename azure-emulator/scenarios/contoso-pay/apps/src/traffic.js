// Invented customers buying from invented merchants. Northwind Airways orders go
// through its storefront (so traces start in the browser-facing service); the
// other merchants call the gateway directly from their own back ends. Refunds,
// chargebacks and the occasional card-testing attack happen on their own.
const { metrics } = require('@opentelemetry/api');
const azure = require('./lib/azure');
const cfg = require('./lib/config');
const log = require('./lib/log');

const TPS = Number(cfg.opt('TRAFFIC_TPS', '1.5'));
const meter = metrics.getMeter('contoso-pay.traffic');
const mSent = meter.createCounter('cpay.traffic.requests', { description: 'Synthetic requests sent' });

const FIRST = ['Amelia', 'Oliver', 'Isla', 'George', 'Ava', 'Noah', 'Mia', 'Arthur', 'Freya', 'Leo', 'Sofia', 'Omar', 'Priya', 'Chen', 'Fatima', 'Lukas', 'Aoife', 'Mateo', 'Zara', 'Kwame', 'Ingrid', 'Ravi', 'Hana', 'Diego'];
const LAST = ['Smith', 'Jones', 'Taylor', 'Brown', 'Williams', 'Patel', 'Khan', 'Murphy', 'Kelly', 'Nowak', 'Rossi', 'Schmidt', 'Okafor', 'Nguyen', 'Silva', 'Andersson', 'Byrne', 'Haddad', 'Mensah', 'Lopez'];
const GOOD = ['4242424242424242', '4000056655665556', '5555555555554444', '2223003122003222', '378282246310005'];
const BAD = ['4000000000000002', '4000000000009995', '4000000000000069', '4000000000000127'];
const pick = (a) => a[Math.floor(Math.random() * a.length)];
const between = (lo, hi) => Math.round(lo + Math.random() * (hi - lo));

function customer(country = 'GB') {
  const name = `${pick(FIRST)} ${pick(LAST)}`;
  return { name, email: `${name.toLowerCase().replace(/[^a-z]+/g, '.')}@example.com`, ip_country: Math.random() < 0.06 ? pick(['US', 'FR', 'NG', 'BR', 'IN']) : country };
}
const card = (number) => ({ number, expiry: '12/29', cvc: number.startsWith('37') ? '1234' : '123' });
const cardNumber = () => (Math.random() < 0.88 ? pick(GOOD) : Math.random() < 0.15 ? '4100000000000019' : pick(BAD));

const keys = {};
const key = async (m) => (keys[m] ||= await azure.getSecret(cfg.merchantVault(), `${m}-api-key`));
const recent = []; // captured payments eligible for refunds / disputes

async function northwind() {
  const flights = ['NW 101', 'NW 214', 'NW 330', 'NW 452', 'NW 508', 'NW 777'];
  const cabin = Math.random() < 0.75 ? 'Economy' : Math.random() < 0.6 ? 'Premium' : 'Business';
  const c = customer('GB');
  const r = await fetch(`${cfg.storefrontUrl()}/api/checkout`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ flight: pick(flights), cabin, passengers: Math.random() < 0.7 ? 1 : between(2, 4), passenger: c, card: card(cardNumber()), ipCountry: c.ip_country }),
  });
  const out = await r.json();
  return { merchant: 'northwind-airways', payment: out.payment };
}

async function direct(merchant, lo, hi, currency, country) {
  const c = customer(country);
  const r = await fetch(`${cfg.gatewayUrl()}/v1/payments`, {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${await key(merchant)}` },
    body: JSON.stringify({ amount: between(lo, hi), currency, reference: `ORD-${between(100000, 999999)}`, card: card(cardNumber()), customer: c }),
  });
  return { merchant, payment: await r.json() };
}

async function one() {
  const roll = Math.random();
  const res = roll < 0.55 ? await northwind()
    : roll < 0.8 ? await direct('fabrikam-hotels', 8900, 89000, 'GBP', 'GB')
      : await direct('tailspin-toys', 499, 14999, 'EUR', 'IE');
  mSent.add(1, { kind: 'payment', merchant: res.merchant });
  if (res.payment?.status === 'captured') {
    recent.push({ ...res, at: Date.now() });
    if (recent.length > 500) recent.shift();
  }
}

async function refundOrDispute() {
  const old = recent.filter((r) => Date.now() - r.at > 20_000);
  if (!old.length) return;
  const victim = old[Math.floor(Math.random() * old.length)];
  recent.splice(recent.indexOf(victim), 1);
  const p = victim.payment;
  if (Math.random() < 0.85) {
    const partial = Math.random() < 0.4;
    await fetch(`${cfg.gatewayUrl()}/v1/payments/${p.id}/refunds`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${await key(victim.merchant)}` },
      body: JSON.stringify({ amount: partial ? Math.max(1, Math.floor(p.amount / 3)) : undefined, reason: pick(['requested_by_customer', 'duplicate', 'flight_cancelled', 'item_not_received']) }),
    });
    mSent.add(1, { kind: 'refund', merchant: victim.merchant });
  } else {
    await fetch(`${cfg.gatewayUrl()}/v1/payments/${p.id}/chargebacks`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason_code: pick(['10.4', '13.1', '13.3', '4837', '4853']) }),
    });
    mSent.add(1, { kind: 'chargeback', merchant: victim.merchant });
  }
}

// A fraudster tests stolen cards in small amounts against the toy shop: one
// card, many attempts, foreign IP. Velocity rules should block most of them.
async function cardTestingAttack() {
  log.warn('card-testing attack started against tailspin-toys');
  for (let i = 0; i < 18; i++) {
    await fetch(`${cfg.gatewayUrl()}/v1/payments`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${await key('tailspin-toys')}` },
      body: JSON.stringify({ amount: between(100, 300), currency: 'EUR', reference: `ORD-${between(100000, 999999)}`, card: card('4000002760003184'), customer: { name: 'Test Buyer', email: 'buyer@example.net', ip_country: 'RU' } }),
    }).catch(() => {});
    mSent.add(1, { kind: 'attack', merchant: 'tailspin-toys' });
    await new Promise((r) => setTimeout(r, 300));
  }
}

const safe = (fn) => () => fn().catch((err) => log.warn({ err: err.message }, 'synthetic request failed'));
setInterval(safe(one), Math.max(50, 1000 / TPS));
setInterval(safe(refundOrDispute), 7000);
setInterval(safe(cardTestingAttack), Number(cfg.opt('ATTACK_EVERY_MS', '240000')));
log.info({ tps: TPS }, 'traffic generator started');
