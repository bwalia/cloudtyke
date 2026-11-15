// Resource names come from the Terraform outputs (written to .env by
// scripts/up.sh, or a ConfigMap on AKS). Credentials never do: those are read
// from Key Vault at runtime.
const need = (k) => {
  const v = process.env[k];
  if (!v) throw new Error(`missing environment variable ${k}`);
  return v;
};
const opt = (k, d) => process.env[k] || d;

module.exports = {
  need,
  opt,
  paymentsVault: () => need('PAYMENTS_VAULT_URI'),
  merchantVault: () => need('MERCHANT_VAULT_URI'),
  receiptsAccount: () => need('RECEIPTS_ACCOUNT'),
  settlementAccount: () => need('SETTLEMENT_ACCOUNT'),
  gatewayUrl: () => opt('GATEWAY_URL', 'http://gateway:8080'),
  riskUrl: () => opt('RISK_URL', 'http://risk-engine:8080'),
  storefrontUrl: () => opt('STOREFRONT_URL', 'http://storefront:8080'),
  jaegerUiUrl: () => opt('JAEGER_UI_URL', 'http://localhost:4731'),
  grafanaUrl: () => opt('GRAFANA_URL', 'http://localhost:4730'),
  port: () => Number(opt('PORT', '8080')),
  merchants: {
    'northwind-airways': { name: 'Northwind Airways', mcc: '4511', country: 'GB', currency: 'GBP', webhook: 'storefront' },
    'fabrikam-hotels': { name: 'Fabrikam Hotels', mcc: '7011', country: 'GB', currency: 'GBP' },
    'tailspin-toys': { name: 'Tailspin Toys', mcc: '5945', country: 'IE', currency: 'EUR' },
  },
};
