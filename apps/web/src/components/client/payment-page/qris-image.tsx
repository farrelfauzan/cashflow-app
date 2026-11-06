"use client";

type QrisImageProps = {
  src: string;
  alt: string;
  // The image did not load (410 qris_expired, or the rate limit): read the status once.
  onError: () => void;
};

// The QR drawn by the API (getPaymentQRImage), same-origin, 190px.
export function QrisImage({ src, alt, onError }: QrisImageProps) {
  return (
    // eslint-disable-next-line @next/next/no-img-element -- The token is in this image's path. next/image's optimizer would put that path, token and all, in a /_next/image?url= query string and fetch it from the server; the image is a fresh no-store PNG per QR, so there is nothing to optimize either.
    <img
      src={src}
      alt={alt}
      width={190}
      height={190}
      decoding="async"
      referrerPolicy="no-referrer"
      onError={onError}
      className="size-[190px] rounded-md bg-card"
    />
  );
}
