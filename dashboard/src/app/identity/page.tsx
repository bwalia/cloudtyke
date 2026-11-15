import type { Metadata } from "next";
import { Suspense } from "react";

import { Identity } from "@/views/identity";

export const metadata: Metadata = { title: "Identity" };

export default function Page() {
  return (
    <Suspense>
      <Identity />
    </Suspense>
  );
}
