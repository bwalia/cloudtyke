# www.cloudtyke.dev

The CloudTyke website: Next.js 16, exported as static files (`output: "export"`) and published with GitHub Pages by
[`.github/workflows/website.yml`](../.github/workflows/website.yml) on every push to `main` that touches `website/`.

```bash
npm ci
npm run dev        # http://localhost:4320
npm run build      # static site in out/
```

Fonts match the Workstation website: Poppins (body), Unbounded (headings) and JetBrains Mono (code), self-hosted by
`next/font` at build time.

## Domains

| Name | DNS (Cloudflare, DNS only) | Served by |
|---|---|---|
| `www.cloudtyke.dev` | CNAME `bwalia.github.io` | GitHub Pages (`public/CNAME`) |
| `cloudtyke.dev` | A/AAAA to GitHub Pages | GitHub, redirects to www |

GitHub Pages issues the certificate, so these two records must stay unproxied (grey cloud). Everything else under
`cloudtyke.dev` is the hosted sandbox (see `azure-emulator/deploy/k8s`).
