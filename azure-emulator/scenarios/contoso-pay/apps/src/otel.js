// Loaded with `node --require` before anything else so the auto-instrumentations
// can patch http, express, pg, mongodb, undici (fetch) and pino.
// Exports traces, metrics and logs over OTLP/HTTP to the collector
// (OTEL_EXPORTER_OTLP_ENDPOINT); OTEL_SERVICE_NAME and
// OTEL_RESOURCE_ATTRIBUTES describe the service.
const { NodeSDK } = require('@opentelemetry/sdk-node');
const { getNodeAutoInstrumentations } = require('@opentelemetry/auto-instrumentations-node');
const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-proto');
const { OTLPMetricExporter } = require('@opentelemetry/exporter-metrics-otlp-proto');
const { OTLPLogExporter } = require('@opentelemetry/exporter-logs-otlp-proto');
const { PeriodicExportingMetricReader } = require('@opentelemetry/sdk-metrics');
const { BatchLogRecordProcessor } = require('@opentelemetry/sdk-logs');

if (process.env.OTEL_SDK_DISABLED !== 'true') {
  const sdk = new NodeSDK({
    traceExporter: new OTLPTraceExporter(),
    metricReader: new PeriodicExportingMetricReader({ exporter: new OTLPMetricExporter(), exportIntervalMillis: 5000 }),
    logRecordProcessors: [new BatchLogRecordProcessor(new OTLPLogExporter())],
    instrumentations: [getNodeAutoInstrumentations({
      '@opentelemetry/instrumentation-fs': { enabled: false },
      '@opentelemetry/instrumentation-dns': { enabled: false },
      '@opentelemetry/instrumentation-net': { enabled: false },
      '@opentelemetry/instrumentation-http': { ignoreIncomingRequestHook: (req) => req.url === '/healthz' },
    })],
  });
  sdk.start();
  const stop = () => sdk.shutdown().finally(() => process.exit(0));
  process.on('SIGTERM', stop);
  process.on('SIGINT', stop);
}
