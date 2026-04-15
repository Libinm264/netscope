import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-semibold font-mono tracking-wide",
  {
    variants: {
      variant: {
        http: "bg-blue-500/20 text-blue-300",
        https: "bg-sky-500/20 text-sky-300",
        dns: "bg-purple-500/20 text-purple-300",
        tls: "bg-emerald-500/20 text-emerald-300",
        tcp: "bg-gray-500/20 text-gray-300",
        udp: "bg-cyan-500/20 text-cyan-300",
        error: "bg-red-500/20 text-red-300",
        default: "bg-white/10 text-gray-300",
      },
    },
    defaultVariants: { variant: "default" },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge, badgeVariants };
