import { useState, useRef, useEffect } from 'react'

export interface DateRange {
  from: string // 'YYYY-MM-DD' or ''
  to: string   // 'YYYY-MM-DD' or ''
}

export const EMPTY_RANGE: DateRange = { from: '', to: '' }

/** Returns true when an ISO timestamp falls within the range (both ends inclusive). */
export function inDateRange(isoDate: string, range: DateRange): boolean {
  if (!range.from && !range.to) return true
  const d = isoDate.slice(0, 10)
  if (range.from && d < range.from) return false
  if (range.to   && d > range.to)   return false
  return true
}

type Preset = '7d' | '30d' | '90d' | '1y' | 'all'

const PRESETS: { key: Preset; label: string; days: number | null }[] = [
  { key: '7d',  label: '7D',  days: 7 },
  { key: '30d', label: '30D', days: 30 },
  { key: '90d', label: '90D', days: 90 },
  { key: '1y',  label: '1Y',  days: 365 },
  { key: 'all', label: 'ALL', days: null },
]

const MONTHS = [
  'January','February','March','April','May','June',
  'July','August','September','October','November','December',
]
const DOW = ['Su','Mo','Tu','We','Th','Fr','Sa']

function toDateStr(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function CalendarMonth({
  year, month, range, hoverDay,
  onDayClick, onDayHover, onPrev, onNext,
}: {
  year: number
  month: number
  range: DateRange
  hoverDay: string | null
  onDayClick: (d: string) => void
  onDayHover: (d: string | null) => void
  onPrev: () => void
  onNext: () => void
}) {
  const today = toDateStr(new Date())
  const numDays = new Date(year, month + 1, 0).getDate()
  const startDow = new Date(year, month, 1).getDay()

  const from = range.from
  const effectiveTo = range.to || (from && hoverDay ? hoverDay : '')
  const [rangeStart, rangeEnd] = from && effectiveTo
    ? (from <= effectiveTo ? [from, effectiveTo] : [effectiveTo, from])
    : [from, '']

  const cells: (number | null)[] = []
  for (let i = 0; i < startDow; i++) cells.push(null)
  for (let d = 1; d <= numDays; d++) cells.push(d)
  while (cells.length % 7 !== 0) cells.push(null)

  return (
    <div className="p-3 select-none" style={{ minWidth: '260px' }}>
      <div className="flex items-center justify-between mb-3">
        <button
          onClick={onPrev}
          className="w-7 h-7 flex items-center justify-center rounded-md text-[var(--text-mid)] hover:bg-[var(--border)]/50 transition-colors"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"><polyline points="15 18 9 12 15 6"/></svg>
        </button>
        <span className="text-sm font-semibold text-[var(--text)]">{MONTHS[month]} {year}</span>
        <button
          onClick={onNext}
          className="w-7 h-7 flex items-center justify-center rounded-md text-[var(--text-mid)] hover:bg-[var(--border)]/50 transition-colors"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"><polyline points="9 18 15 12 9 6"/></svg>
        </button>
      </div>

      <div className="grid grid-cols-7 mb-1">
        {DOW.map(d => (
          <div key={d} className="text-center text-[10px] font-medium text-[var(--text-light)] py-1 tracking-wide">{d}</div>
        ))}
      </div>

      <div className="grid grid-cols-7 gap-y-0.5">
        {cells.map((day, i) => {
          if (!day) return <div key={i} />
          const str = `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`
          const isFrom = str === from
          const isTo   = str === range.to
          const isEndpoint = isFrom || isTo
          const inRange = !!(rangeStart && rangeEnd && str > rangeStart && str < rangeEnd)
          const isToday = str === today

          return (
            <button
              key={i}
              onClick={() => onDayClick(str)}
              onMouseEnter={() => onDayHover(str)}
              onMouseLeave={() => onDayHover(null)}
              className={[
                'relative h-8 w-full text-sm transition-colors focus:outline-none',
                isEndpoint
                  ? 'bg-[var(--gold)] text-[var(--cream)] font-semibold rounded-md'
                  : inRange
                  ? 'bg-[var(--gold)]/15 text-[var(--text)]'
                  : 'text-[var(--text-mid)] hover:bg-[var(--border)]/50 rounded-md',
                !isEndpoint && isToday ? 'font-semibold !text-[var(--gold)]' : '',
                isFrom && rangeEnd ? 'rounded-r-none' : '',
                isTo && rangeStart   ? 'rounded-l-none' : '',
              ].filter(Boolean).join(' ')}
            >
              {day}
              {isToday && !isEndpoint && (
                <span className="absolute bottom-1 left-1/2 -translate-x-1/2 w-1 h-1 rounded-full bg-[var(--gold)]" />
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}

export function DateRangePicker({
  value,
  onChange,
}: {
  value: DateRange
  onChange: (r: DateRange) => void
}) {
  const [activePreset, setActivePreset] = useState<Preset | null>('all')
  const [open, setOpen]       = useState(false)
  const [viewYear, setViewYear]   = useState(new Date().getFullYear())
  const [viewMonth, setViewMonth] = useState(new Date().getMonth())
  const [hoverDay, setHoverDay]   = useState<string | null>(null)
  const [step, setStep] = useState<'from' | 'to'>('from')
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handle = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [open])

  function applyPreset(key: Preset, days: number | null) {
    setActivePreset(key)
    setOpen(false)
    setHoverDay(null)
    if (days === null) {
      onChange(EMPTY_RANGE)
    } else {
      const now  = new Date()
      const from = new Date(now)
      from.setDate(from.getDate() - days)
      onChange({ from: toDateStr(from), to: toDateStr(now) })
    }
  }

  function handleDayClick(d: string) {
    if (step === 'from') {
      onChange({ from: d, to: '' })
      setStep('to')
      setActivePreset(null)
    } else {
      const [from, to] = value.from <= d ? [value.from, d] : [d, value.from]
      onChange({ from, to })
      setStep('from')
      setOpen(false)
      setHoverDay(null)
    }
  }

  const isCustom = activePreset === null
  const displayLabel = isCustom && value.from
    ? (value.to ? `${value.from.slice(5)} – ${value.to.slice(5)}` : `${value.from.slice(5)} – …`)
    : 'CUSTOM'

  const btnBase   = 'px-2.5 py-1 text-xs font-deco tracking-wide border transition-colors'
  const btnActive = 'bg-[var(--text)] text-[var(--cream)] border-[var(--text)]'
  const btnIdle   = 'bg-transparent text-[var(--text-light)] border-[var(--border)] hover:border-[var(--border-dark)] hover:text-[var(--text-mid)]'

  return (
    <div className="relative flex items-center gap-1.5 flex-wrap" ref={containerRef}>
      <span className="text-xs font-deco tracking-widest text-[var(--text-light)] uppercase mr-0.5">Period</span>

      {PRESETS.map(p => (
        <button
          key={p.key}
          onClick={() => applyPreset(p.key, p.days)}
          className={`${btnBase} ${activePreset === p.key ? btnActive : btnIdle}`}
        >
          {p.label}
        </button>
      ))}

      <button
        onClick={() => { setOpen(o => !o); setStep('from') }}
        className={`${btnBase} ${isCustom || open ? btnActive : btnIdle}`}
      >
        {displayLabel}
      </button>

      {isCustom && (value.from || value.to) && (
        <button
          onClick={() => { onChange(EMPTY_RANGE); setActivePreset('all') }}
          className="text-[var(--text-light)] hover:text-[var(--text-mid)] text-xs transition-colors"
          title="Clear"
        >✕</button>
      )}

      {open && (
        <div
          className="absolute top-full left-0 mt-2 z-50 bg-[var(--cream)] border border-[var(--border)] rounded-xl overflow-hidden"
          style={{ boxShadow: 'var(--shadow-lg)' }}
        >
          <div className="px-4 pt-3 pb-1 text-xs font-medium"
               style={{ color: step === 'from' ? 'var(--text-light)' : 'var(--gold)' }}>
            {step === 'from' ? 'Select start date' : 'Now select end date'}
          </div>
          <CalendarMonth
            year={viewYear}
            month={viewMonth}
            range={value}
            hoverDay={step === 'to' ? hoverDay : null}
            onDayClick={handleDayClick}
            onDayHover={setHoverDay}
            onPrev={() => {
              if (viewMonth === 0) { setViewYear(y => y - 1); setViewMonth(11) }
              else setViewMonth(m => m - 1)
            }}
            onNext={() => {
              if (viewMonth === 11) { setViewYear(y => y + 1); setViewMonth(0) }
              else setViewMonth(m => m + 1)
            }}
          />
        </div>
      )}
    </div>
  )
}
