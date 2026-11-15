import type { Metadata } from "next";
import { Suspense } from "react";

import { Network } from "@/views/network";

export const metadata: Metadata = { title: "Network" };

export default function Page() {
  return (
    <Suspense>
      <Network />
    </Suspense>
  );
}
