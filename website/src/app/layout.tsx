import type { Metadata, Viewport } from "next";
import { Poppins, Unbounded, JetBrains_Mono } from "next/font/google";
import "./globals.css";

// Same type system as the Workstation website: Poppins for body and UI,
// Unbounded for display headings, JetBrains Mono for code.
const poppins = Poppins({
  subsets: ["latin"],
  display: "swap",
  variable: "--ct-font-body",
  weight: ["400", "500", "600", "700"],
});
const unbounded = Unbounded({
  subsets: ["latin"],
  display: "swap",
  variable: "--ct-font-display",
  weight: ["500", "700", "800"],
});
const mono = JetBrains_Mono({
  subsets: ["latin"],
  display: "swap",
  variable: "--ct-font-mono",
  weight: ["400", "500", "700"],
});

export const metadata: Metadata = {
  metadataBase: new URL("https://www.cloudtyke.dev"),
  title: "CloudTyke: a small cloud for big mistakes",
  description:
    "CloudTyke emulates Azure's APIs (ARM, Entra ID, Graph, RBAC, Policy, Key Vault, Storage, AKS) in Docker. " +
    "Point Terraform or OpenTofu at it: no subscription, no bill, no blast radius.",
  openGraph: {
    title: "CloudTyke: a small cloud for big mistakes",
    description: "Learn and test Azure infrastructure as code against a local emulator, free and safe.",
    url: "https://www.cloudtyke.dev",
    siteName: "CloudTyke",
    type: "website",
  },
  alternates: { canonical: "https://www.cloudtyke.dev/" },
};

export const viewport: Viewport = { themeColor: "#0b1020" };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${poppins.variable} ${unbounded.variable} ${mono.variable}`}>
      <body>{children}</body>
    </html>
  );
}
