interface StatusBadgeProps {
  status: string;
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  const isActive = status === "active";
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-[11px] font-medium ${
        isActive ? "bg-green-50 text-green-700" : "bg-gray-100 text-gray-500"
      }`}
    >
      {status}
    </span>
  );
}
