import { NextResponse, type NextRequest } from "next/server";

/**
 * Optional password gate for LAN use. When CLOUDTYKE_DASHBOARD_PASSWORD is set, every page and API
 * route needs HTTP Basic auth (any user name, that password). The console acts as the emulator's
 * seeded admin identities, so set this whenever it listens on anything other than 127.0.0.1.
 */
export function proxy(req: NextRequest) {
  const password = process.env.CLOUDTYKE_DASHBOARD_PASSWORD;
  if (!password) return NextResponse.next();
  const header = req.headers.get("authorization") ?? "";
  if (header.startsWith("Basic ")) {
    try {
      const decoded = atob(header.slice(6));
      const given = decoded.slice(decoded.indexOf(":") + 1);
      if (safeEqual(given, password)) return NextResponse.next();
    } catch {
      /* malformed header: fall through to 401 */
    }
  }
  return new NextResponse("Authentication required", {
    status: 401,
    headers: { "www-authenticate": 'Basic realm="CloudTyke console", charset="UTF-8"' },
  });
}

function safeEqual(a: string, b: string) {
  let diff = a.length ^ b.length;
  for (let i = 0; i < Math.max(a.length, b.length); i++) diff |= (a.charCodeAt(i) || 0) ^ (b.charCodeAt(i) || 0);
  return diff === 0;
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
