"use client";

import { useAdminEmail } from "@/context/AdminSessionContext";
import ChangePasswordForm from "@/components/ChangePasswordForm";

export default function AdminPage() {
  const email = useAdminEmail();

  return (
    <div className="max-w-xl">
      <h1 className="text-lg font-serif mb-4">Account</h1>

      <div className="bg-white rounded-lg shadow-sm p-4 mb-4">
        <p className="text-xs text-gray-400">Signed in as</p>
        <p className="text-sm font-medium">{email}</p>
      </div>

      <div className="bg-white rounded-lg shadow-sm p-4">
        <h2 className="text-sm font-medium mb-1">Change password</h2>
        <p className="text-xs text-gray-500 mb-4">
          Enter your current password and choose a new one.
        </p>
        <ChangePasswordForm email={email} />
      </div>
    </div>
  );
}
