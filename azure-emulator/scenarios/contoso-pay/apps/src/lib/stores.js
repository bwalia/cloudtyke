// Connections to the payment data stores. The host, password and Mongo
// connection string are read from the payments Key Vault, exactly as they would
// be for Azure Database for PostgreSQL and Cosmos DB for MongoDB.
const { Pool } = require('pg');
const { MongoClient } = require('mongodb');
const azure = require('./azure');
const cfg = require('./config');
const log = require('./log');

let pool;
async function pg() {
  if (pool) return pool;
  const vault = cfg.paymentsVault();
  const [host, password] = await Promise.all([azure.getSecret(vault, 'postgres-host'), azure.getSecret(vault, 'postgres-admin-password')]);
  pool = new Pool({
    host, password, user: cfg.opt('PGUSER', 'cpayadmin'), database: cfg.opt('PGDATABASE', 'payments'), port: 5432, max: 10,
    // Azure requires TLS; the local stand-in container does not offer it.
    ssl: process.env.PGSSL_DISABLE === 'true' ? false : { rejectUnauthorized: false },
  });
  pool.on('error', (err) => log.error({ err: err.message }, 'postgres pool error'));
  await pool.query('select 1');
  log.info({ host }, 'connected to PostgreSQL');
  return pool;
}

let mongo;
async function events() {
  if (mongo) return mongo;
  const conn = await azure.getSecret(cfg.paymentsVault(), 'cosmos-mongo-connection');
  const opts = { serverSelectionTimeoutMS: 5000 };
  if (process.env.MONGO_LOCAL_STANDIN === 'true') {
    // The local mongo container stands in for Cosmos DB: no TLS, no "globaldb" replica set.
    Object.assign(opts, { tls: false, directConnection: true, replicaSet: undefined, authSource: 'admin' });
  }
  const client = new MongoClient(conn.replace(/([?&])(ssl|replicaSet)=[^&]*&?/g, '$1'), opts);
  await client.connect();
  mongo = client.db('payments');
  log.info('connected to Cosmos DB (MongoDB API)');
  return mongo;
}

module.exports = { pg, events };
