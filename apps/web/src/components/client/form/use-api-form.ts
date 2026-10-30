"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { flushSync } from "react-dom";
import {
  useForm,
  type DefaultValues,
  type FieldValues,
  type Path,
} from "react-hook-form";
import { z } from "zod";
import { id } from "zod/locales";

import { toFormError } from "@/lib/api/form-error";

// A check without its own message (none should be) still speaks Bahasa Indonesia.
z.config(id());

type ApiFormOptions<Input extends FieldValues, Output> = {
  // The friendly early checks, with their messages. The Go API checks everything again and its
  // message always wins.
  schema: z.ZodType<Output, Input>;
  defaultValues: DefaultValues<Input>;
  // The fields of this form an API error can name ("name", "address"); the form's field names
  // are the API's. An error naming any other field, or none, goes to the alert.
  apiFields: readonly Path<Input>[];
};

// The one way a panel form is set up with react-hook-form:
// - the schema runs on submit only; a failed check puts its message under its field and focuses
//   the first field with an error;
// - a field's error goes away as soon as the field is edited and is checked again on the next
//   submit, never while typing;
// - a refused request puts the API's message under the field it names (and focuses it), or in
//   the alert at the top of the form.
export function useApiForm<Input extends FieldValues, Output>({
  schema,
  defaultValues,
  apiFields,
}: ApiFormOptions<Input, Output>) {
  const form = useForm<Input, unknown, Output>({
    resolver: zodResolver(schema),
    defaultValues,
    mode: "onSubmit",
    reValidateMode: "onSubmit",
  });
  const { clearErrors, setError, setFocus } = form;

  // Registers a text field so that editing it clears its error.
  function field(name: Path<Input>) {
    return form.register(name, { onChange: () => clearErrors(name) });
  }

  function showApiError(error: unknown) {
    const failed = toFormError(error, apiFields);
    const apiField = apiFields.find((name) => name === failed.field);
    if (apiField === undefined) {
      setError("root.server", { type: "server", message: failed.message });
      return;
    }
    // The mutation's error arrives while the fields are still disabled for saving, and a
    // disabled field cannot take focus: render the error (and the enabled fields) first.
    flushSync(() =>
      setError(apiField, { type: "server", message: failed.message }),
    );
    setFocus(apiField);
  }

  return {
    form,
    field,
    showApiError,
    // The alert at the top of the form; handleSubmit clears it on every submit.
    alert: form.formState.errors.root?.server?.message,
  };
}
