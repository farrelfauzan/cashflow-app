import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Slot } from "radix-ui";

const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-base font-medium whitespace-nowrap transition-all duration-100 outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background active:scale-[0.97] disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 motion-reduce:active:scale-100 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        // Variants by meaning (T-093, board UI-Tombol): default creates, saves or confirms;
        // caution changes an amount or a date already billed or agreed; destructive is hard to
        // undo. The outline ones open such an action, the solid ones confirm it. Hover and
        // pressed turn to the -strong shade (pressed also shrinks 3%, not with reduced motion);
        // every pair's contrast is noted in globals.css.
        default:
          "bg-primary text-primary-foreground hover:bg-primary-strong active:bg-primary-strong",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive-strong focus-visible:ring-destructive active:bg-destructive-strong dark:bg-destructive/60 dark:focus-visible:ring-destructive/40",
        "destructive-outline":
          "border border-destructive bg-background text-destructive shadow-xs hover:bg-destructive hover:text-destructive-foreground focus-visible:border-destructive focus-visible:ring-destructive active:border-destructive-strong active:bg-destructive-strong active:text-destructive-foreground",
        caution:
          "bg-warning text-warning-foreground hover:bg-warning-strong focus-visible:ring-warning active:bg-warning-strong",
        "caution-outline":
          "border border-warning bg-background text-warning shadow-xs hover:bg-warning hover:text-warning-foreground focus-visible:border-warning focus-visible:ring-warning active:border-warning-strong active:bg-warning-strong active:text-warning-foreground",
        outline:
          "border bg-background shadow-xs hover:bg-secondary hover:text-secondary-foreground active:bg-secondary dark:border-input dark:bg-input/30 dark:hover:bg-input/50",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost:
          "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50",
        link: "text-primary underline-offset-4 hover:underline",
      },
      // Touch targets are at least 44px (h-11) at every size, CLAUDE.md "UX rules". The
      // upstream xs sizes (24px) are removed; sm differs from default only in padding.
      size: {
        default: "h-11 px-5 py-2 has-[>svg]:px-4",
        sm: "h-11 gap-1.5 rounded-md px-3 has-[>svg]:px-2.5",
        lg: "h-12 rounded-md px-6 has-[>svg]:px-5",
        icon: "size-11",
        "icon-sm": "size-11",
        "icon-lg": "size-12",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
  }) {
  const Comp = asChild ? Slot.Root : "button";

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  );
}

export { Button, buttonVariants };
