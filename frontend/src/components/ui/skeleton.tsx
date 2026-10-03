import type { HTMLAttributes } from "react";
import { cn } from "@/lib/utils";
import { TableCell, TableRow } from "@/components/ui/table";

export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("animate-pulse rounded-md bg-muted", className)} {...props} />;
}

// TableSkeletonRows renders shimmer rows inside an existing <TableBody>.
export function TableSkeletonRows({ cols, rows = 5 }: { cols: number; rows?: number }) {
  return (
    <>
      {Array.from({ length: rows }).map((_, r) => (
        <TableRow key={r} className="hover:bg-transparent">
          <TableCell colSpan={cols} className="py-3">
            <div className="flex items-center gap-4">
              {Array.from({ length: Math.min(cols, 5) }).map((_, c) => (
                <Skeleton key={c} className={cn("h-4", c === 0 ? "w-40" : "flex-1")} />
              ))}
            </div>
          </TableCell>
        </TableRow>
      ))}
    </>
  );
}

// CardGridSkeleton mimics a responsive grid of cards.
export function CardGridSkeleton({ cards = 6 }: { cards?: number }) {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: cards }).map((_, i) => (
        <div key={i} className="space-y-3 rounded-lg border bg-card p-4">
          <Skeleton className="h-5 w-1/2" />
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-2/3" />
        </div>
      ))}
    </div>
  );
}

// ListSkeleton is a stack of row-shaped placeholders (mobile card lists).
export function ListSkeleton({ rows = 6 }: { rows?: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-4 rounded-lg border bg-card p-4">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 flex-1" />
          <Skeleton className="h-4 w-20" />
        </div>
      ))}
    </div>
  );
}
