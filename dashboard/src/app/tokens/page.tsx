import type { Metadata } from "next";
import { Suspense } from "react";

import { Tokens } from "@/views/tokens";

export const metadata: Metadata = { title: "Tokens & OIDC" };

export default function Page() {
  return (
    <Suspense>
      <Tokens />
    </Suspense>
  );
}
