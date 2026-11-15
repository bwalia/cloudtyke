import type { Metadata } from "next";

import { Kubernetes } from "@/views/kubernetes";

export const metadata: Metadata = { title: "Kubernetes" };

export default function Page() {
  return <Kubernetes />;
}
