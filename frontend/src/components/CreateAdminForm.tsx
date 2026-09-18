"use client";

import { useState, type FormEvent } from "react";
import { createAdminSchema } from "@/lib/validation";
import { fieldErrorsFrom } from "@/lib/zodErrors";
import { createAdmin } from "@/lib/api";
import PasswordField from "@/components/PasswordField";

interface CreateAdminFormProps {
  onCreated: () => void;
}

export default function CreateAdminForm({ onCreated }: CreateAdminFormProps) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    setFieldErrors({});

    const result = createAdminSchema.safeParse({ email, password });
    if (!result.success) {
      setFieldErrors(fieldErrorsFrom(result.error));
      return;
    }

    setLoading(true);
    try {
      await createAdmin(result.data.email, result.data.password);
      onCreated();
    } catch (err) {
      setFormError(
        err instanceof Error && err.message === "email_already_exists"
          ? "An admin with this email already exists"
          : "Could not create admin",
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <div>
        <label className="block text-xs font-medium mb-1" htmlFor="email">
          Email
        </label>
        <input
          id="email"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
        />
        {fieldErrors.email && <p className="text-xs text-red-600 mt-1">{fieldErrors.email}</p>}
      </div>
      <PasswordField
        id="password"
        label="Password"
        autoComplete="new-password"
        value={password}
        onChange={setPassword}
        error={fieldErrors.password}
      />
      {formError && <p className="text-xs text-red-600">{formError}</p>}
      <button
        type="submit"
        disabled={loading}
        className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium disabled:opacity-50 w-full"
      >
        {loading ? "Creating..." : "Create admin"}
      </button>
    </form>
  );
}
