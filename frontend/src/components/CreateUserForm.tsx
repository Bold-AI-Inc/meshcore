"use client";

import { useState, type FormEvent } from "react";
import { createUserSchema } from "@/lib/validation";
import { fieldErrorsFrom } from "@/lib/zodErrors";
import { createUser, type CreateUserResult } from "@/lib/api";

interface CreateUserFormProps {
  onCreated: (result: CreateUserResult) => void;
}

export default function CreateUserForm({ onCreated }: CreateUserFormProps) {
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    setFieldErrors({});

    const result = createUserSchema.safeParse({ email, displayName });
    if (!result.success) {
      setFieldErrors(fieldErrorsFrom(result.error));
      return;
    }

    setLoading(true);
    try {
      const created = await createUser(result.data.email, result.data.displayName);
      onCreated(created);
    } catch (err) {
      setFormError(
        err instanceof Error && err.message === "email_already_exists"
          ? "A user with this email already exists"
          : "Could not create user",
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <div>
        <label className="block text-xs font-medium mb-1" htmlFor="displayName">
          Name
        </label>
        <input
          id="displayName"
          value={displayName}
          onChange={(event) => setDisplayName(event.target.value)}
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
        />
        {fieldErrors.displayName && (
          <p className="text-xs text-red-600 mt-1">{fieldErrors.displayName}</p>
        )}
      </div>
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
      {formError && <p className="text-xs text-red-600">{formError}</p>}
      <button
        type="submit"
        disabled={loading}
        className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium disabled:opacity-50 w-full"
      >
        {loading ? "Creating..." : "Create user"}
      </button>
    </form>
  );
}
