interface PaginationProps {
  page: number;
  totalPages: number;
  onChange: (page: number) => void;
}

export default function Pagination({ page, totalPages, onChange }: PaginationProps) {
  if (totalPages <= 1) {
    return null;
  }

  return (
    <div className="flex items-center justify-end gap-2 pt-3 mt-3 border-t border-gray-100">
      <button
        type="button"
        onClick={() => onChange(page - 1)}
        disabled={page <= 1}
        className="px-2 py-1 text-xs rounded-md border border-gray-300 disabled:opacity-40"
      >
        Prev
      </button>
      <span className="text-xs text-gray-500">
        Page {page} of {totalPages}
      </span>
      <button
        type="button"
        onClick={() => onChange(page + 1)}
        disabled={page >= totalPages}
        className="px-2 py-1 text-xs rounded-md border border-gray-300 disabled:opacity-40"
      >
        Next
      </button>
    </div>
  );
}
