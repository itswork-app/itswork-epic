import { NextResponse } from "next/server";
import { parse } from "tldts";
import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";

// Define public routes that don't require authentication
const isPublicRoute = createRouteMatcher([
  "/",
  "/sign-in(.*)",
  "/sign-up(.*)",
  "/api/webhook/helius",
]);

export default clerkMiddleware(async (auth, request) => {
  const url = request.nextUrl.clone();
  const hostname = request.headers.get('host') || '';
  const parsed = parse(hostname);
  const subdomain = parsed.subdomain;

  // 1. Hostname Rewrites (PR-NEXUS-SUBDOMAIN-ORCHESTRATION)
  if (subdomain === 'trader') {
    if (url.pathname === '/' || url.pathname === '') {
      url.pathname = '/dashboard/trader';
      return NextResponse.rewrite(url);
    }
  }

  if (subdomain === 'dev' || subdomain === 'developer') {
    if (url.pathname === '/' || url.pathname === '') {
      url.pathname = '/dashboard/developer';
      return NextResponse.rewrite(url);
    }
  }

  // 2. Auth Protection
  const isTeaserRequest = request.nextUrl.pathname.startsWith("/api/v1/token") && 
                          request.nextUrl.searchParams.get("teaser") === "true";

  if (!isPublicRoute(request) && !isTeaserRequest) {
    await auth.protect();
  }

  return NextResponse.next();
});

export const config = {
  matcher: [
    // Skip Next.js internals and all static files, unless found in search params
    "/((?!_next|[^?]*\\.(?:html|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)",
    // Always run for API routes
    "/(api|trpc)(.*)",
  ],
};
