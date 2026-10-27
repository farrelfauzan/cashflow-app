import { Card } from "@/components/ui/card";

export function PlaceholderNotice() {
  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-12">
      <Card className="w-full max-w-md gap-3 px-6 py-8">
        <h1 className="text-2xl font-semibold tracking-tight">
          Pengelolaan Sewa
        </h1>
        <p className="text-base text-muted-foreground">
          Aplikasi sedang disiapkan.
        </p>
      </Card>
    </main>
  );
}
