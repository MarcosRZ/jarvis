import { cn } from "cn";

export function Logo({
  className,
  compact = false,
}: {
  className?: string;
  compact?: boolean;
}) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <div className="flex size-10 shrink-0 items-center justify-center border-2 border-foreground bg-primary font-mono text-sm font-bold tracking-tighter text-primary-foreground">
        M4
      </div>
      {!compact && (
        <div className="flex min-w-0 flex-col">
          <span className="truncate text-sm font-semibold tracking-tight">
            M4nhattan
          </span>
          <span className="truncate text-xs text-muted-foreground">
            Service control
          </span>
        </div>
      )}
    </div>
  );
}
