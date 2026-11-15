import type { Metadata } from "next";

import { Tour } from "@/views/tour";

export const metadata: Metadata = { title: "Guided tour" };

export default function Page() {
  return <Tour />;
}
