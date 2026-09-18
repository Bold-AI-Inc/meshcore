import { useState } from "react";

export function usePaginatedFilter<T>(
  items: T[] | null | undefined,
  filterFn: (item: T, query: string) => boolean,
  pageSize = 10,
) {
  const [search, setSearchState] = useState("");
  const [page, setPage] = useState(1);

  const safeItems = items ?? [];

  const query = search.trim().toLowerCase();
  const filtered = query ? safeItems.filter((item) => filterFn(item, query)) : safeItems;

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(page, totalPages);
  const pageItems = filtered.slice((safePage - 1) * pageSize, safePage * pageSize);

  function setSearch(value: string) {
    setSearchState(value);
    setPage(1);
  }

  return {
    search,
    setSearch,
    page: safePage,
    setPage,
    totalPages,
    pageItems,
    totalCount: filtered.length,
  };
}
