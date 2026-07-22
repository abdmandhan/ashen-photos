/** @type {import('next').NextConfig} */
const nextConfig = {
  // Thumbnails are presigned MinIO URLs on arbitrary hosts; use plain <img>.
  images: { unoptimized: true },
};

export default nextConfig;
