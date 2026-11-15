import type { Metadata } from "next";

import { Network } from "@/views/network";

export const metadata: Metadata = { title: "Network" };

export default function Page() {
  return <Network />;
}
