// Nightly settlement, run every few minutes for the demo (team: Data & Settlement).
// Nets captured payments per merchant, takes Contoso Pay's fee, writes the
// settlement file to the Azure Files share the acquirers and the finance team
// read, and a reconciliation summary next to it.
const crypto = require('crypto');
const { trace, metrics } = require('@opentelemetry/api');
const azure = require('./lib/azure');
const stores = require('./lib/stores');
const cfg = require('./lib/config');
const log = require('./lib/log');

const tracer = trace.getTracer('contoso-pay.settlement');
const meter = metrics.getMeter('contoso-pay.settlement');
const mBatches = meter.createCounter('cpay.settlement.batches', { description: 'Settlement batches written' });
const mNet = meter.createCounter('cpay.settlement.net', { description: 'Net amount settled to merchants', unit: '{minor}' });
const INTERVAL = Number(cfg.opt('SETTLEMENT_INTERVAL_MS', '120000'));
const FEE = { 'northwind-airways': [0.0145, 20], 'fabrikam-hotels': [0.0165, 20], 'tailspin-toys': [0.019, 25] }; // percentage + fixed (minor units)

async function run() {
  await tracer.startActiveSpan('settlement.run', async (span) => {
    try {
      const db = await stores.pg();
      const account = cfg.settlementAccount();
      const day = new Date().toISOString().slice(0, 10);
      let batches = 0;
      for (const mid of Object.keys(cfg.merchants)) {
        const { rows } = await db.query(
          `select id, reference, amount_minor, refunded_minor, currency, card_brand, card_last4, auth_code, captured_at
           from payments where merchant_id=$1 and status in ('captured','partially_refunded','refunded') and settled_batch is null
           order by captured_at limit 5000`, [mid]);
        if (!rows.length) continue;
        const batch = `stl_${day.replace(/-/g, '')}_${crypto.randomBytes(3).toString('hex')}`;
        const [pct, fixed] = FEE[mid];
        let gross = 0; let refunds = 0; let fees = 0;
        const lines = ['payment_id,reference,captured_at,card,auth_code,currency,gross_minor,refunded_minor,fee_minor,net_minor'];
        for (const r of rows) {
          const g = Number(r.amount_minor); const rf = Number(r.refunded_minor);
          const fee = Math.round(g * pct) + fixed;
          gross += g; refunds += rf; fees += fee;
          lines.push([r.id, `"${(r.reference || '').replace(/"/g, '')}"`, r.captured_at?.toISOString(), `${r.card_brand} ${r.card_last4}`, r.auth_code, r.currency, g, rf, fee, g - rf - fee].join(','));
        }
        const net = gross - refunds - fees;
        const currency = rows[0].currency;
        const file = `${day}/${mid}/${batch}.csv`;
        await azure.putFile(account, 'settlement-outbound', file, lines.join('\n') + '\n');
        await azure.putFile(account, 'reconciliation', `${day}/${batch}.json`, JSON.stringify({ batch, merchant: mid, currency, payments: rows.length, gross_minor: gross, refunds_minor: refunds, fees_minor: fees, net_minor: net, file: `settlement-outbound/${file}` }, null, 2));
        await db.query('begin');
        await db.query('insert into settlement_batches (id,merchant_id,currency,payment_count,gross_minor,refunds_minor,fees_minor,net_minor,file_path) values ($1,$2,$3,$4,$5,$6,$7,$8,$9)',
          [batch, mid, currency, rows.length, gross, refunds, fees, net, file]);
        await db.query('update payments set settled_batch=$1 where id = any($2)', [batch, rows.map((r) => r.id)]);
        await db.query('commit');
        mBatches.add(1, { merchant: mid });
        mNet.add(net, { merchant: mid, currency });
        batches++;
        log.info({ batch, merchant: mid, payments: rows.length, net_minor: net, currency, file }, 'settlement batch written');
      }
      span.setAttribute('cpay.settlement.batches', batches);
    } catch (err) {
      span.recordException(err);
      log.error({ err: err.message }, 'settlement run failed');
      await (await stores.pg()).query('rollback').catch(() => {});
    } finally {
      span.end();
    }
  });
}

azure.retry('payments database', async () => { const db = await stores.pg(); await db.query('select 1 from settlement_batches limit 1'); })
  .then(() => { log.info({ everyMs: INTERVAL }, 'settlement batch scheduler started'); setInterval(run, INTERVAL); setTimeout(run, 15000); })
  .catch((err) => { log.fatal({ err: err.message }, 'settlement failed to start'); process.exit(1); });
