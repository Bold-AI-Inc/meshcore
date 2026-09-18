"use client";

import { useState, type FormEvent } from "react";
import { changePasswordSchema } from "@/lib/validation";
import { fieldErrorsFrom } from "@/lib/zodErrors";
import { changePassword } from "@/lib/api";
import { downloadTextFile } from "@/lib/download";
import PasswordField from "@/components/PasswordField";
import PasswordSaveDialog from "@/components/PasswordSaveDialog";

interface ChangePasswordFormProps {
  email: string;
}

export default function ChangePasswordForm({ email }: ChangePasswordFormProps) {
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [success, setSuccess] = useState(false);
  const [loading, setLoading] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    setSuccess(false);
    setFieldErrors({});

    const result = changePasswordSchema.safeParse({ oldPassword, newPassword, confirmPassword });
    if (!result.success) {
      setFieldErrors(fieldErrorsFrom(result.error));
      return;
    }

    setDialogOpen(true);
  }

  async function commitChange(download: boolean) {
    setLoading(true);
    try {
      await changePassword(oldPassword, newPassword);
      if (download) {
        downloadTextFile(
          "mesh-admin-credentials.txt",
          `Email: ${email}\nPassword: ${newPassword}\nChanged: ${new Date().toISOString()}\n`,
        );
      }
      setSuccess(true);
      setOldPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setDialogOpen(false);
    } catch {
      setFormError("Current password is incorrect");
      setDialogOpen(false);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <form onSubmit={handleSubmit} className="space-y-3">
        <PasswordField
          id="oldPassword"
          label="Current password"
          autoComplete="current-password"
          value={oldPassword}
          onChange={setOldPassword}
          error={fieldErrors.oldPassword}
        />
        <PasswordField
          id="newPassword"
          label="New password"
          autoComplete="new-password"
          value={newPassword}
          onChange={setNewPassword}
          error={fieldErrors.newPassword}
        />
        <PasswordField
          id="confirmPassword"
          label="Confirm new password"
          autoComplete="new-password"
          value={confirmPassword}
          onChange={setConfirmPassword}
          error={fieldErrors.confirmPassword}
        />
        {formError && <p className="text-xs text-red-600">{formError}</p>}
        {success && <p className="text-xs text-green-600">Password updated</p>}
        <button
          type="submit"
          className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium"
        >
          Save password
        </button>
      </form>

      {dialogOpen && (
        <PasswordSaveDialog
          loading={loading}
          onCancel={() => setDialogOpen(false)}
          onSave={() => commitChange(false)}
          onSaveAndDownload={() => commitChange(true)}
        />
      )}
    </>
  );
}
