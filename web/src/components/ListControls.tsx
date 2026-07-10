import type { UsePaginationResult } from '../hooks/usePagination'

interface ListControlsProps<T> {
  pagination: UsePaginationResult<T>
  searchPlaceholder?: string
  showPageSize?: boolean
  pageSizeOptions?: number[]
  className?: string
}

export function ListControls<T>({
  pagination,
  searchPlaceholder = 'Search...',
  showPageSize = true,
  pageSizeOptions = [10, 25, 50, 100],
  className = '',
}: ListControlsProps<T>) {
  const {
    page,
    pageSize,
    totalPages,
    totalItems,
    search,
    setSearch,
    setPage,
    nextPage,
    prevPage,
    setPageSize,
    canNextPage,
    canPrevPage,
    startIndex,
    endIndex,
  } = pagination

  return (
    <div className={`flex flex-wrap items-center justify-between gap-4 ${className}`}>
      {/* Search */}
      <div className="relative flex-1 min-w-[200px] max-w-md">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={searchPlaceholder}
          className="rr-input w-full pl-9 pr-8"
        />
        <svg 
          className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-light)]"
          fill="none" 
          stroke="currentColor" 
          viewBox="0 0 24 24"
        >
          <path 
            strokeLinecap="round" 
            strokeLinejoin="round" 
            strokeWidth={2} 
            d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" 
          />
        </svg>
        {search && (
          <button
            onClick={() => setSearch('')}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-light)] hover:text-[var(--text)] cursor-pointer"
            aria-label="Clear search"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        )}
      </div>

      {/* Right side: Page size + Pagination */}
      <div className="flex items-center gap-4">
        {/* Page size selector */}
        {showPageSize && (
          <div className="flex items-center gap-2">
            <span className="text-[var(--text-light)] text-xs font-deco tracking-wider uppercase">Show</span>
            <select
              value={pageSize}
              onChange={(e) => setPageSize(Number(e.target.value))}
              className="rr-select !py-1.5 !px-3 !pr-8 !text-xs min-w-[70px]"
            >
              {pageSizeOptions.map((size) => (
                <option key={size} value={size}>{size}</option>
              ))}
            </select>
          </div>
        )}

        {/* Pagination info */}
        <span className="text-[var(--text-light)] text-xs tabular-nums">
          {totalItems === 0 ? '0 results' : `${startIndex}–${endIndex} of ${totalItems}`}
        </span>

        {/* Pagination buttons */}
        <div className="flex items-center gap-1">
          <button
            onClick={() => setPage(1)}
            disabled={!canPrevPage}
            className="p-1.5 rounded border border-[var(--border)] text-[var(--text-mid)] disabled:opacity-40 disabled:cursor-not-allowed hover:bg-[var(--cream-alt)] hover:border-[var(--border-dark)] transition-colors cursor-pointer"
            aria-label="First page"
          >
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
            </svg>
          </button>
          <button
            onClick={prevPage}
            disabled={!canPrevPage}
            className="p-1.5 rounded border border-[var(--border)] text-[var(--text-mid)] disabled:opacity-40 disabled:cursor-not-allowed hover:bg-[var(--cream-alt)] hover:border-[var(--border-dark)] transition-colors cursor-pointer"
            aria-label="Previous page"
          >
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </button>
          
          <span className="px-2 text-xs font-mono text-[var(--text)]">
            {page} / {totalPages}
          </span>
          
          <button
            onClick={nextPage}
            disabled={!canNextPage}
            className="p-1.5 rounded border border-[var(--border)] text-[var(--text-mid)] disabled:opacity-40 disabled:cursor-not-allowed hover:bg-[var(--cream-alt)] hover:border-[var(--border-dark)] transition-colors cursor-pointer"
            aria-label="Next page"
          >
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
            </svg>
          </button>
          <button
            onClick={() => setPage(totalPages)}
            disabled={!canNextPage}
            className="p-1.5 rounded border border-[var(--border)] text-[var(--text-mid)] disabled:opacity-40 disabled:cursor-not-allowed hover:bg-[var(--cream-alt)] hover:border-[var(--border-dark)] transition-colors cursor-pointer"
            aria-label="Last page"
          >
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 5l7 7-7 7M5 5l7 7-7 7" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  )
}

// Compact version for use in cards/modals
export function ListControlsCompact<T>({
  pagination,
  searchPlaceholder = 'Search...',
}: Pick<ListControlsProps<T>, 'pagination' | 'searchPlaceholder'>) {
  const {
    page,
    totalPages,
    totalItems,
    search,
    setSearch,
    nextPage,
    prevPage,
    canNextPage,
    canPrevPage,
    startIndex,
    endIndex,
  } = pagination

  return (
    <div className="flex items-center gap-3">
      {/* Search */}
      <div className="relative flex-1">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={searchPlaceholder}
          className="rr-input w-full !py-1.5 !text-xs pl-8"
        />
        <svg 
          className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-[var(--text-light)]"
          fill="none" 
          stroke="currentColor" 
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
      </div>

      {/* Compact pagination */}
      <div className="flex items-center gap-1">
        <button
          onClick={prevPage}
          disabled={!canPrevPage}
          className="p-1 rounded text-[var(--text-mid)] disabled:opacity-40 hover:bg-[var(--cream-alt)] transition-colors cursor-pointer"
        >
          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <span className="text-[10px] text-[var(--text-light)] tabular-nums whitespace-nowrap">
          {page}/{totalPages}
        </span>
        <button
          onClick={nextPage}
          disabled={!canNextPage}
          className="p-1 rounded text-[var(--text-mid)] disabled:opacity-40 hover:bg-[var(--cream-alt)] transition-colors cursor-pointer"
        >
          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
          </svg>
        </button>
      </div>
    </div>
  )
}
