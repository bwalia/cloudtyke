import type { Metadata } from "next";

import { Shell } from "@/components/shell";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "CloudTyke console", template: "%s · CloudTyke" },
  description: "Console for the CloudTyke local Azure emulator: resources, identity, access control, activity and live service tests.",
};

// Applies the saved (or system) theme before first paint, so there is no flash.
const themeScript = `try{var t=localStorage.getItem('ct-theme');if(t==='dark'||(!t&&matchMedia('(prefers-color-scheme: dark)').matches))document.documentElement.classList.add('dark')}catch(e){}`;

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" suppressHydrationWarning className="h-full antialiased">
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="min-h-full">
        <Shell>{children}</Shell>
      </body>
    </html>
  );
}
