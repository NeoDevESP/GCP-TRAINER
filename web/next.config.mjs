/** Static export served by the Go binary (WEB_DIR=web/out). */
const nextConfig = {
  output: "export",
  images: { unoptimized: true },
  trailingSlash: false,
  reactStrictMode: true,
};
export default nextConfig;
