"use client";

import { useState } from "react";
import { downloadTextFile } from "@/lib/download";

interface CopyableKeyRevealProps {
  email: string;
  keyPlaintext: string;
  onDismiss: () => void;
}

export default function CopyableKeyReveal({ email, keyPlaintext, onDismiss }: CopyableKeyRevealProps) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    await navigator.clipboard.writeText(keyPlaintext);
    setCopied(true);
  }

  function handleDownload() {
    downloadTextFile(
      "mesh-user-key.txt",
      `Email: ${email}\nAPI Key: ${keyPlaintext}\nGenerated: ${new Date().toISOString()}\n`,
    );
  }

  return (
    <div className="bg-white rounded-lg shadow-sm p-4 mb-4 border border-amber-200">
      <p className="text-xs font-medium mb-1">API key generated</p>
      <p className="text-xs text-gray-500 mb-2">
        This key is shown once and cannot be retrieved again. Copy or download it now.
      </p>
      <code className="block text-xs bg-gray-50 border border-gray-200 rounded-md px-2 py-1.5 mb-2 break-all">
        {keyPlaintext}
      </code>
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={handleCopy}
          className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium hover:bg-gray-50"
        >
          {copied ? "Copied" : "Copy to clipboard"}
        </button>
        <button
          type="button"
          onClick={handleDownload}
          className="rounded-md border border-gray-300 px-3 py-1.5 text-xs font-medium hover:bg-gray-50"
        >
          Download .txt
        </button>
        <button
          type="button"
          onClick={onDismiss}
          className="ml-auto text-xs text-gray-400 hover:text-gray-600"
        >
          Dismiss
        </button>
      </div>
    </div>
  );
}
