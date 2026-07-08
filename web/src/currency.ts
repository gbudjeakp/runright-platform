import { useEffect, useState } from 'react'

export type CurrencyCode = 'USD' | 'EUR' | 'GBP' | 'JPY' | 'CAD' | 'AUD' | 'CNY' | 'INR'

const STORAGE_KEY = 'rr-currency'

const EXCHANGE_RATES: Record<CurrencyCode, number> = {
  USD: 1.0,
  EUR: 0.92,
  GBP: 0.79,
  JPY: 154.5,
  CAD: 1.37,
  AUD: 1.52,
  CNY: 7.24,
  INR: 83.12,
}

export const CURRENCY_OPTIONS: Array<{ code: CurrencyCode; label: string }> = [
  { code: 'USD', label: 'US Dollar (USD)' },
  { code: 'EUR', label: 'Euro (EUR)' },
  { code: 'GBP', label: 'British Pound (GBP)' },
  { code: 'JPY', label: 'Japanese Yen (JPY)' },
  { code: 'CAD', label: 'Canadian Dollar (CAD)' },
  { code: 'AUD', label: 'Australian Dollar (AUD)' },
  { code: 'CNY', label: 'Chinese Yuan (CNY)' },
  { code: 'INR', label: 'Indian Rupee (INR)' },
]

function isCurrencyCode(value: string): value is CurrencyCode {
  return value in EXCHANGE_RATES
}

export function getStoredCurrency(): CurrencyCode {
  if (typeof window === 'undefined') return 'USD'
  const raw = localStorage.getItem(STORAGE_KEY) ?? 'USD'
  return isCurrencyCode(raw) ? raw : 'USD'
}

export function useCurrencyPreference() {
  const [currency, setCurrencyState] = useState<CurrencyCode>(() => getStoredCurrency())

  useEffect(() => {
    if (typeof window === 'undefined') return
    const onStorage = (event: StorageEvent) => {
      if (event.key !== STORAGE_KEY) return
      setCurrencyState(getStoredCurrency())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const setCurrency = (next: CurrencyCode) => {
    setCurrencyState(next)
    if (typeof window !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, next)
    }
  }

  return { currency, setCurrency }
}

export function formatFromUSD(
  amountUSD: number,
  currency: CurrencyCode,
  opts?: { minimumFractionDigits?: number; maximumFractionDigits?: number },
): string {
  const converted = amountUSD * EXCHANGE_RATES[currency]
  const zeroDecimal = currency === 'JPY'
  const minimumFractionDigits = opts?.minimumFractionDigits ?? (zeroDecimal ? 0 : 2)
  const maximumFractionDigits = opts?.maximumFractionDigits ?? (zeroDecimal ? 0 : 2)

  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(converted)
}

export function convertFromUSD(amountUSD: number, currency: CurrencyCode): number {
  return amountUSD * EXCHANGE_RATES[currency]
}

export function convertToUSD(amountInCurrency: number, currency: CurrencyCode): number {
  const rate = EXCHANGE_RATES[currency]
  if (!Number.isFinite(amountInCurrency) || !Number.isFinite(rate) || rate <= 0) return 0
  return amountInCurrency / rate
}
