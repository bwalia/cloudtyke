import type { Metadata } from "next";
import { Suspense } from "react";

import { Services } from "@/views/services";

export const metadata: Metadata = { title: "Services" };

export default function Page() {
  return (
    <Suspense>
      <Services />
    </Suspense>
  );
}
