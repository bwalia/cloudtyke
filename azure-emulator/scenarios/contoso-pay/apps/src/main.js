// One image, one entrypoint per service: SERVICE selects which one runs.
const services = {
  storefront: './storefront',
  gateway: './gateway',
  'risk-engine': './risk',
  'processor-portal': './portal',
  'traffic-generator': './traffic',
  'settlement-batch': './settlement',
};
const mod = services[process.env.SERVICE];
if (!mod) {
  console.error(`SERVICE must be one of: ${Object.keys(services).join(', ')}`);
  process.exit(2);
}
require(mod);
