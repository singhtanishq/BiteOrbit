import type { NextConfig } from "next";

const apiOrigin = process.env.API_ORIGIN ?? "http://127.0.0.1:8080";

/**
 * All API calls stay same-origin via a rewrite proxy so the Go API's
 * HttpOnly session cookies work without cross-site CORS gymnastics.
 */
const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination: `${apiOrigin}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
