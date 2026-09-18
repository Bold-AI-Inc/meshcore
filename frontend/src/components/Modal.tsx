"use client";

import type { ReactNode } from "react";

interface ModalProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
  size?: "sm" | "lg";
}

const SIZE_CLASS = { sm: "max-w-sm", lg: "max-w-2xl" };

export default function Modal({ title, onClose, children, size = "sm" }: ModalProps) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <div className={`bg-white rounded-lg p-5 w-full ${SIZE_CLASS[size]}`}>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-sm font-medium">{title}</h3>
          <button
            type="button"
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 text-sm leading-none"
          >
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}
