import Image from "next/image";
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
      <div className="size-10 shrink-0 overflow-hidden border-2 border-foreground bg-primary">
        <Image
          src="/doctor-manhattan.jpg"
          alt=""
          width={80}
          height={80}
          priority
          className="size-full object-cover"
        />
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
