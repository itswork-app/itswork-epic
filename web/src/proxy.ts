import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";

// Define public routes that don't require authentication
const isPublicRoute = createRouteMatcher([
  "/",
  "/sign-in(.*)",
  "/sign-up(.*)",
  "/api/webhook/helius",
]);

export default clerkMiddleware(async (auth, request) => {
  const isTeaserRequest = request.nextUrl.pathname.startsWith("/api/v1/token") && 
                          request.nextUrl.searchParams.get("teaser") === "true";

  if (!isPublicRoute(request) && !isTeaserRequest) {
    // Protect all other routes
    await auth.protect();
  }
});

export const config = {
  matcher: [
    // Skip Next.js internals and all static files, unless found in search params
    "/((?!_next|[^?]*\\.(?:html|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)",
    // Always run for API routes
    "/(api|trpc)(.*)",
  ],
};
