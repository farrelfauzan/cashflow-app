import { AppMark } from "./app-mark";
import { WORDMARK_LEAD, WORDMARK_TAIL } from "./utils";

// The logo and the name, as on the boards Login (36px mark, 16px name), Login-iPad and
// Organizations (40px, 20px).
export function AppBrand() {
  return (
    <div className="flex items-center gap-2.5 md:gap-3">
      <AppMark className="size-9 md:size-10" />
      <span className="text-base font-bold tracking-[-0.02em] md:text-xl">
        {WORDMARK_LEAD}
        <span className="text-primary">{WORDMARK_TAIL}</span>
      </span>
    </div>
  );
}
