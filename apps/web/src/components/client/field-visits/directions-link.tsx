"use client";

import { MapPin } from "lucide-react";

import { Button } from "@/components/ui/button";

import { DIRECTIONS_TEXT } from "./utils";

type DirectionsLinkProps = {
  // The maps search for the unit's address (directionsUrl).
  href: string;
};

// "Arahkan": opens the unit's address in the phone's maps app, in a new tab. A plain link: the
// page loads nothing from the maps site, and sends it no referrer.
export function DirectionsLink({ href }: DirectionsLinkProps) {
  return (
    <Button
      asChild
      variant="outline"
      className="h-12 flex-1 rounded-xl border-[1.5px] bg-card px-4 text-base font-semibold"
    >
      <a href={href} target="_blank" rel="noopener noreferrer">
        <MapPin aria-hidden="true" className="size-5" />
        {DIRECTIONS_TEXT}
      </a>
    </Button>
  );
}
