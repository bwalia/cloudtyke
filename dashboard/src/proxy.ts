import { NextResponse, type NextRequest } from "next/server";

/**
 * Optional sign-in gate for LAN use. When CLOUDTYKE_DASHBOARD_PASSWORD is set, every page and API
 * route needs HTTP Basic auth with that password, and with CLOUDTYKE_DASHBOARD_USER as the user
 * name when that is set too. The console acts as the emulator's
 * seeded admin identities, so set this whenever it listens on anything other than 127.0.0.1.
 */
export function proxy(req: NextRequest) {
  const password = process.env.CLOUDTYKE_DASHBOARD_PASSWORD;
  const user = process.env.CLOUDTYKE_DASHBOARD_USER;
  if (!password) return NextResponse.next();
  const header = req.headers.get("authorization") ?? "";
  if (header.startsWith("Basic ")) {
    try {
      const decoded = atob(header.slice(6));
      const sep = decoded.indexOf(":");
      const givenUser = decoded.slice(0, sep);
      const given = decoded.slice(sep + 1);
      const userOk = !user || safeEqual(givenUser, user);
      if (sep >= 0 && safeEqual(given, password) && userOk) return NextResponse.next();
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
