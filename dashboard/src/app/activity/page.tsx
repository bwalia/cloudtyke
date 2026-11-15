import type { Metadata } from "next";
import { Suspense } from "react";

import { Activity } from "@/views/activity";

export const metadata: Metadata = { title: "Activity" };

export default function Page() {
  return (
    <Suspense>
      <Activity />
    </Suspense>
  );
}
