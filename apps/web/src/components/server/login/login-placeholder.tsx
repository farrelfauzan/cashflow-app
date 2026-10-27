import { Card } from "@/components/ui/card";
import { APP_NAME } from "@/lib/app-name";

// Temporary: the real login screen comes in W3 from its approved board.
export function LoginPlaceholder() {
  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-12">
      <Card className="w-full max-w-md gap-3 px-6 py-8">
        <h1 className="text-2xl font-semibold tracking-tight">{APP_NAME}</h1>
        <p className="text-base text-muted-foreground">
          Halaman masuk sedang disiapkan.
        </p>
      </Card>
    </main>
  );
}
