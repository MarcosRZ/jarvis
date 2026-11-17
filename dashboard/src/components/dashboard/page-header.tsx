import { Badge } from "@/components/ui/badge";
import { cn } from "cn";

const badgeStyles: Record<string, string> = {
  operational: "border-emerald-500/30 text-emerald-600 dark:text-emerald-400",
  operativo: "border-emerald-500/30 text-emerald-600 dark:text-emerald-400",
  degraded: "border-amber-500/30 text-amber-600 dark:text-amber-400",
  unavailable: "border-destructive/30 text-destructive",
  loading: "border-border text-muted-foreground",
};

export function PageHeader({
  title,
  description,
  badge,
  children,
}: {
  title: string;
  description: string;
  badge?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          {badge && (
            <Badge
              variant="outline"
              className={cn(
                "rounded-none text-[10px]",
                badgeStyles[badge] ?? "border-border text-muted-foreground",
              )}
            >
              {badge}
            </Badge>
          )}
        </div>
        <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>
      </div>
      {children && <div className="flex items-center gap-2">{children}</div>}
    </div>
  );
}
