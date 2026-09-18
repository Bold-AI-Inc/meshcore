"use client";

import { useState } from "react";

interface PasswordSaveDialogProps {
  onCancel: () => void;
  onSave: () => void;
  onSaveAndDownload: () => void;
  loading: boolean;
}

const CONFIRM_WORD = "confirm";

export default function PasswordSaveDialog({
  onCancel,
  onSave,
  onSaveAndDownload,
  loading,
}: PasswordSaveDialogProps) {
  const [text, setText] = useState("");
  const confirmed = text.trim().toLowerCase() === CONFIRM_WORD;

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-lg p-5 w-full max-w-sm">
        <h3 className="text-sm font-medium mb-1">Confirm password change</h3>
        <p className="text-xs text-gray-500 mb-3">
          Type <span className="font-medium">confirm</span> below to save your new password.
        </p>
        <input
          autoFocus
          value={text}
          onChange={(event) => setText(event.target.value)}
          placeholder="confirm"
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500 mb-4"
        />
        <div className="flex items-center justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={loading}
            className="px-3 py-1.5 text-xs font-medium rounded-md text-gray-600 hover:bg-gray-100 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onSave}
            disabled={!confirmed || loading}
            className="px-3 py-1.5 text-xs font-medium rounded-md border border-gray-300 disabled:opacity-40"
          >
            Save
          </button>
          <button
            type="button"
            onClick={onSaveAndDownload}
            disabled={!confirmed || loading}
            className="px-3 py-1.5 text-xs font-medium rounded-md bg-black text-white disabled:opacity-40"
          >
            Save & Download
          </button>
        </div>
      </div>
    </div>
  );
}
