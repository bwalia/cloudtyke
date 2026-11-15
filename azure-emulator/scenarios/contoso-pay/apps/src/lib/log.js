// pino logs carry trace_id/span_id (injected by the pino instrumentation) and
// are also shipped to the collector as OpenTelemetry log records.
const pino = require('pino');
module.exports = pino({ name: process.env.SERVICE, level: process.env.LOG_LEVEL || 'info', base: { service: process.env.SERVICE } });
