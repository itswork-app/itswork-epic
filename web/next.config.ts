import { withSentryConfig } from "@sentry/nextjs";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/webhook/helius",
        destination: `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/webhook/helius`,
      },
      {
        source: "/api/:path*",
        destination: `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/:path*`,
      },
    ];
  },
};

export default withSentryConfig(nextConfig, {
  org: "itswork",
  project: "itswork-app",
  silent: true,
  widenClientFileUpload: true,
  disableLogger: true,
});
