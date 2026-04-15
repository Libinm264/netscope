import * as React from "react";
import { cn } from "@/lib/utils";

const Input = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ className, type, ...props }, ref) => (
  <input
    type={type}
    className={cn(
      "flex h-8 w-full rounded-md border border-white/20 bg-white/5 px-3 py-1 text-sm text-white shadow-sm transition-colors",
      "placeholder:text-gray-500",
      "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-blue-500",
      "disabled:cursor-not-allowed disabled:opacity-50",
      className
    )}
    ref={ref}
    {...props}
  />
));
Input.displayName = "Input";

export { Input };
