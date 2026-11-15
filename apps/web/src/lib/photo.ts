// Photos people send from their phone: the tenant's transfer proof now, the karyawan's receipt
// photo next. Before a photo leaves the phone it is made small enough to send over a weak signal:
// the longest side at most PHOTO_MAX_SIDE, re-encoded as JPEG. An iPhone's HEIC photo becomes a
// JPEG the same way, where the browser can decode it. A photo the browser cannot decode is sent as
// it is, and the API, which judges the file by its content, answers for it. Nothing here reads or
// sends anything but the chosen file.

// The API's limit (upload.MaxSize, 10 MiB), checked here too so a photo too large is refused
// before it is sent: the API closes the connection on a body that is too large, which a browser
// may report as a failed connection instead of the API's message.
export const PHOTO_MAX_BYTES = 10 << 20;

// The longest side of a photo that is re-encoded, in pixels: a bank app's screen or a receipt
// stays readable.
export const PHOTO_MAX_SIDE = 2000;

export const PHOTO_JPEG_QUALITY = 0.85;

// A photo already in a type the API takes, within PHOTO_MAX_SIDE and no larger than this, is sent
// as it is: re-encoding it would only lose detail.
export const PHOTO_KEEP_BYTES = 1 << 20;

// The types the API takes (sendPaymentProof): JPEG, PNG and WebP.
export const PHOTO_TYPES: readonly string[] = [
  "image/jpeg",
  "image/png",
  "image/webp",
];

export type PhotoSize = { width: number; height: number };

// The size a photo is drawn at: scaled down so its longest side is at most maxSide, keeping its
// shape, never scaled up, in whole pixels and at least 1 by 1.
export function fitWithin(size: PhotoSize, maxSide: number): PhotoSize {
  const longest = Math.max(size.width, size.height);
  const scale = longest > maxSide ? maxSide / longest : 1;
  return {
    width: Math.max(1, Math.round(size.width * scale)),
    height: Math.max(1, Math.round(size.height * scale)),
  };
}

// Whether a decoded photo is sent as it is (see PHOTO_KEEP_BYTES).
export function keepsOriginal(photo: {
  type: string;
  bytes: number;
  size: PhotoSize;
}): boolean {
  return (
    PHOTO_TYPES.includes(photo.type) &&
    photo.bytes <= PHOTO_KEEP_BYTES &&
    Math.max(photo.size.width, photo.size.height) <= PHOTO_MAX_SIDE
  );
}

// The re-encoded photo's file name: the original's with .jpg in place of its extension
// ("IMG_0012.HEIC" gives "IMG_0012.jpg"), or "foto.jpg" when it has none to keep.
export function jpegName(name: string): string {
  const base = name.replace(/\.[^./\\]*$/, "").trim();
  return `${base === "" ? "foto" : base}.jpg`;
}

// Whether the photo is too large to send (see PHOTO_MAX_BYTES).
export function isTooLarge(bytes: number): boolean {
  return bytes > PHOTO_MAX_BYTES;
}

async function decode(file: Blob): Promise<ImageBitmap | null> {
  if (typeof createImageBitmap !== "function") return null;
  try {
    // A phone camera's photo is turned upright by its orientation tag.
    return await createImageBitmap(file, { imageOrientation: "from-image" });
  } catch {
    return null;
  }
}

function encode(bitmap: ImageBitmap, size: PhotoSize): Promise<Blob | null> {
  const canvas = document.createElement("canvas");
  canvas.width = size.width;
  canvas.height = size.height;
  const context = canvas.getContext("2d");
  if (context === null) return Promise.resolve(null);
  // A transparent PNG would turn black in a JPEG: it is drawn on white.
  context.fillStyle = "white";
  context.fillRect(0, 0, size.width, size.height);
  context.drawImage(bitmap, 0, 0, size.width, size.height);
  return new Promise((resolve) => {
    canvas.toBlob(resolve, "image/jpeg", PHOTO_JPEG_QUALITY);
  });
}

// The file to send for a chosen photo: the photo itself when it is small enough or cannot be
// decoded here, otherwise a JPEG of it at most PHOTO_MAX_SIDE on its longest side. Never throws.
export async function preparePhoto(file: File): Promise<File> {
  const bitmap = await decode(file);
  if (bitmap === null) return file;
  try {
    const size = { width: bitmap.width, height: bitmap.height };
    if (keepsOriginal({ type: file.type, bytes: file.size, size })) {
      return file;
    }
    const blob = await encode(bitmap, fitWithin(size, PHOTO_MAX_SIDE));
    if (blob === null) return file;
    return new File([blob], jpegName(file.name), { type: "image/jpeg" });
  } catch {
    return file;
  } finally {
    bitmap.close();
  }
}
