import type { Metadata } from "next";
import { Suspense } from "react";

import { Emulator } from "@/views/emulator";

export const metadata: Metadata = { title: "Emulator" };

export default function Page() {
  return (
    <Suspense>
      <Emulator />
    </Suspense>
  );
}
