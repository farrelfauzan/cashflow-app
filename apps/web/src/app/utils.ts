import { Plus_Jakarta_Sans } from "next/font/google";

// Downloaded at build time and served from this app, so pages make no request to Google.
export const plusJakartaSans = Plus_Jakarta_Sans({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-plus-jakarta-sans",
});
