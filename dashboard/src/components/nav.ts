import { Activity, Boxes, Compass, Container, FolderTree, KeyRound, LayoutDashboard, Network, ServerCog, ShieldCheck, Users } from "lucide-react";

export const NAV = [
  { href: "/", label: "Overview", icon: LayoutDashboard, key: "o", group: "Home" },
  { href: "/services", label: "Services", icon: Boxes, key: "s", group: "Home" },
  { href: "/tour", label: "Guided tour", icon: Compass, key: "t", group: "Home" },
  { href: "/resources", label: "Resources", icon: FolderTree, key: "r", group: "Manage" },
  { href: "/network", label: "Network", icon: Network, key: "n", group: "Manage" },
  { href: "/kubernetes", label: "Kubernetes", icon: Container, key: "c", group: "Manage" },
  { href: "/identity", label: "Identity", icon: Users, key: "i", group: "Manage" },
  { href: "/access", label: "Access control", icon: ShieldCheck, key: "a", group: "Manage" },
  { href: "/activity", label: "Activity", icon: Activity, key: "l", group: "Monitor" },
  { href: "/tokens", label: "Tokens & OIDC", icon: KeyRound, key: "k", group: "Monitor" },
  { href: "/emulator", label: "Emulator", icon: ServerCog, key: "e", group: "Monitor" },
] as const;
