import type { Metadata } from "next";

import { Access } from "@/views/access";

export const metadata: Metadata = { title: "Access control" };

export default function Page() {
  return <Access />;
}
