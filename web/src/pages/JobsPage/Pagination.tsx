import { pgBtnStyle, pageNumbers } from './utils'
import { PAGE_SIZES } from './types'

interface PaginationProps {
  page: number
  pageSize: number
  total: number
  onPageChange: (page: number) => void
  onPageSizeChange: (size: number) => void
}

export default function Pagination({ page, pageSize, total, onPageChange, onPageSizeChange }: PaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-2">
        <span className="font-deco text-[12px] tracking-[1.5px] text-[var(--text-light)]">ROWS</span>
        {PAGE_SIZES.map((s) => (
          <button key={s} onClick={() => onPageSizeChange(s)} style={pgBtnStyle(false, pageSize === s)}>
            {s}
          </button>
        ))}
      </div>
      <div className="flex items-center gap-1.5 flex-wrap">
        <span className="font-deco text-[12px] tracking-[1px] text-[var(--text-light)] mr-2">
          {(page - 1) * pageSize + 1}–{Math.min(page * pageSize, total)} of {total}
        </span>
        <button onClick={() => onPageChange(1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>
          «
        </button>
        <button onClick={() => onPageChange(page - 1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>
          ‹
        </button>
        {pageNumbers(page, totalPages).map((p, i) =>
          p === null ? (
            <span key={`e-${i}`} className="text-[var(--text-light)] px-1 font-deco">
              …
            </span>
          ) : (
            <button key={p} onClick={() => onPageChange(p)} style={pgBtnStyle(false, p === page)}>
              {p}
            </button>
          )
        )}
        <button
          onClick={() => onPageChange(page + 1)}
          disabled={page === totalPages}
          style={pgBtnStyle(page === totalPages)}
        >
          ›
        </button>
        <button
          onClick={() => onPageChange(totalPages)}
          disabled={page === totalPages}
          style={pgBtnStyle(page === totalPages)}
        >
          »
        </button>
      </div>
    </div>
  )
}
