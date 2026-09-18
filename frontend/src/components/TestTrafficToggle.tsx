"use client";

interface TestTrafficToggleProps {
  checked: boolean;
  onChange: (next: boolean) => void;
}

export default function TestTrafficToggle({ checked, onChange }: TestTrafficToggleProps) {
  return (
    <label className="flex items-center gap-1.5 text-xs text-gray-500 cursor-pointer select-none">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="accent-black"
      />
      Include test traffic
    </label>
  );
}
