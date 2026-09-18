interface SearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}

export default function SearchInput({ value, onChange, placeholder }: SearchInputProps) {
  return (
    <input
      value={value}
      onChange={(event) => onChange(event.target.value)}
      placeholder={placeholder ?? "Search..."}
      className="rounded-md border border-gray-300 px-3 py-1.5 text-xs outline-none focus:border-gray-500 w-56"
    />
  );
}
