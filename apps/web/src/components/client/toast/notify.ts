import { createElement } from "react";
import { toast } from "sonner";

import { AppToast } from "./app-toast";
import type { ToastMessage } from "./type";
import { toastDuration } from "./utils";

// Shows a toast. Call it only once the API has answered with success (a mutation's onSuccess),
// never before; errors stay in the form. Nothing here is logged. Its own file, not utils.ts,
// because it renders AppToast, which reads utils.ts.
export function notify(message: ToastMessage): void {
  toast.custom((id) => createElement(AppToast, { id, ...message }), {
    duration: toastDuration(message.tone),
  });
}
