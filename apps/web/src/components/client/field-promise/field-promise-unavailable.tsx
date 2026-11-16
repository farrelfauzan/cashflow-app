"use client";

type FieldPromiseUnavailableProps = {
  // The API's message (a 403 or 404, already Bahasa Indonesia) or NO_FIELD_PROMISE_TEXT.
  message: string;
  className: string;
};

// "Catat janji bayar" when no promise can be recorded: the reason, in place of the fields.
export function FieldPromiseUnavailable({
  message,
  className,
}: FieldPromiseUnavailableProps) {
  return (
    <div className={className}>
      <p role="alert" className="text-base">
        {message}
      </p>
    </div>
  );
}
