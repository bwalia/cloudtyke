import type { Metadata } from "next";
import { JetBrains_Mono, Poppins, Unbounded } from "next/font/google";

import { Shell } from "@/components/shell";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "CloudTyke console", template: "%s · CloudTyke" },
  description: "Console for the CloudTyke local Azure emulator: resources, identity, access control, activity and live service tests.",
};

// Same type system as the CloudTyke website and the Workstation website:
// Poppins for text, Unbounded for page titles, JetBrains Mono for code and IDs.
const poppins = Poppins({ subsets: ["latin"], display: "swap", variable: "--ct-font-body", weight: ["400", "500", "600", "700"] });
const unbounded = Unbounded({ subsets: ["latin"], display: "swap", variable: "--ct-font-display", weight: ["600", "700"] });
const mono = JetBrains_Mono({ subsets: ["latin"], display: "swap", variable: "--ct-font-mono", weight: ["400", "500", "700"] });

// Applies the saved (or system) theme before first paint, so there is no flash.
const themeScript = `try{var t=localStorage.getItem('ct-theme');if(t==='dark'||(!t&&matchMedia('(prefers-color-scheme: dark)').matches))document.documentElement.classList.add('dark')}catch(e){}`;

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" suppressHydrationWarning className={`h-full antialiased ${poppins.variable} ${unbounded.variable} ${mono.variable}`}>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="min-h-full">
        <Shell>{children}</Shell>
      </body>
    </html>
  );
}
