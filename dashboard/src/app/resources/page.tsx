import type { Metadata } from "next";
import { Suspense } from "react";

import { Resources } from "@/views/resources";

export const metadata: Metadata = { title: "Resources" };

export default function Page() {
  return (
    <Suspense>
      <Resources />
    </Suspense>
  );
}
