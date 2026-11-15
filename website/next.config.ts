import type { NextConfig } from "next";

// Static export for GitHub Pages (www.cloudtyke.dev). No server features.
const nextConfig: NextConfig = {
  output: "export",
  trailingSlash: true,
  poweredByHeader: false,
  images: { unoptimized: true },
};

export default nextConfig;
