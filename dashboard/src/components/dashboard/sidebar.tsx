"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Bell,
  LayoutDashboard,
  ScrollText,
  Server,
  Settings,
} from "lucide-react";
import { cn } from "cn";
import { Badge } from "@/components/ui/badge";
import { Logo } from "@/components/dashboard/logo";

const primaryNav = [
  { label: "General", href: "/", icon: LayoutDashboard },
  { label: "Servicios", href: "/servicios", icon: Server },
  { label: "Registros", href: "/registros", icon: ScrollText },
  {
    label: "Notificaciones",
    href: "/notificaciones",
    icon: Bell,
    badge: "3",
  },
];

const secondaryNav = [
  { label: "Configuracion", href: "/configuracion", icon: Settings, soon: true },
];

function NavItem({
  label,
  href,
  icon: Icon,
  badge,
  soon,
}: {
  label: string;
  href: string;
  icon: typeof Server;
  badge?: string;
  soon?: boolean;
}) {
  const pathname = usePathname();
  const active = href === "/" ? pathname === "/" : pathname.startsWith(href);

  const className = cn(
    "group flex items-center gap-3 px-3 py-2.5 text-sm font-medium transition-colors",
    active
      ? "border-l-2 border-primary bg-sidebar-accent text-sidebar-accent-foreground"
      : "border-l-2 border-transparent text-sidebar-foreground/80 hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
    soon && "cursor-not-allowed opacity-50 hover:bg-transparent",
  );

  const content = (
    <>
      <Icon className="size-4 shrink-0" />
      <span className="flex-1 truncate">{label}</span>
      {badge && (
        <Badge className="h-5 min-w-5 rounded-none px-1.5 text-[10px]">
          {badge}
        </Badge>
      )}
      {soon && (
        <span className="text-[10px] font-normal uppercase tracking-wide text-muted-foreground">
          pronto
        </span>
      )}
    </>
  );

  if (soon) {
    return (
      <span aria-disabled className={className}>
        {content}
      </span>
    );
  }

  return (
    <Link href={href} aria-current={active ? "page" : undefined} className={className}>
      {content}
    </Link>
  );
}

export function SidebarContent() {
  return (
    <div className="flex h-full flex-col bg-sidebar text-sidebar-foreground">
      <div className="flex h-16 items-center border-b border-sidebar-border px-5">
        <Logo />
      </div>

      <nav className="flex flex-1 flex-col gap-8 overflow-y-auto px-3 py-6">
        <div className="space-y-1">
          <p className="px-3 pb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
            Principal
          </p>
          {primaryNav.map((item) => (
            <NavItem key={item.label} {...item} />
          ))}
        </div>

        <div className="space-y-1">
          <p className="px-3 pb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
            Sistema
          </p>
          {secondaryNav.map((item) => (
            <NavItem key={item.label} {...item} />
          ))}
        </div>
      </nav>

      <div className="p-3">
        <div className="border border-sidebar-border bg-background/60 p-3">
          <div className="flex items-center gap-2">
            <span className="relative flex size-2">
              <span className="absolute inline-flex size-full animate-ping bg-emerald-500 opacity-60" />
              <span className="relative inline-flex size-2 bg-emerald-500" />
            </span>
            <span className="text-xs font-semibold">Sistema operativo</span>
          </div>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
            5 de 6 servicios activos. Home Assistant esta detenido.
          </p>
          <div className="mt-3 flex items-center justify-between border-t border-sidebar-border pt-2 text-[10px] uppercase tracking-wide text-muted-foreground">
            <span>v0.1.0</span>
            <span>host: mini-pc</span>
          </div>
        </div>
      </div>
    </div>
  );
}

export function AppSidebar() {
  return (
    <aside className="fixed inset-y-0 left-0 z-40 hidden w-64 border-r border-sidebar-border lg:block">
      <SidebarContent />
    </aside>
  );
}
