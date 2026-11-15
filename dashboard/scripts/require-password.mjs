// Refuses to expose the console on the network without a password (see src/proxy.ts).
if (!process.env.CLOUDTYKE_DASHBOARD_PASSWORD) {
  console.error("Set CLOUDTYKE_DASHBOARD_PASSWORD before listening on the LAN, e.g.\n  CLOUDTYKE_DASHBOARD_PASSWORD='pick-something' npm run start:lan");
  process.exit(1);
}
