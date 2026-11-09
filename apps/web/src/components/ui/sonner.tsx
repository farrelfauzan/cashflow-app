"use client";

import { Toaster as Sonner, type ToasterProps } from "sonner";

// shadcn's Sonner wrapper. Changed from the CLI's output: the app has one light theme (no
// next-themes), and every toast is drawn by components/client/toast/app-toast.tsx, so the
// default icons and colors are not set here.
const Toaster = ({ ...props }: ToasterProps) => {
  return <Sonner theme="light" className="toaster group" {...props} />;
};

export { Toaster };
