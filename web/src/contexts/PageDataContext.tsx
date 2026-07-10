import React, { createContext, useContext, useState, useCallback } from 'react'
import type { PageContext } from '../types'

interface PageDataContextValue {
  pageData: Record<string, unknown>
  setPageData: (data: Record<string, unknown>) => void
  clearPageData: () => void
}

const PageDataContext = createContext<PageDataContextValue>({
  pageData: {},
  setPageData: () => {},
  clearPageData: () => {},
})

export function PageDataProvider({ children }: { children: React.ReactNode }) {
  const [pageData, setPageDataState] = useState<Record<string, unknown>>({})

  const setPageData = useCallback((data: Record<string, unknown>) => {
    setPageDataState(data)
  }, [])

  const clearPageData = useCallback(() => {
    setPageDataState({})
  }, [])

  return (
    <PageDataContext.Provider value={{ pageData, setPageData, clearPageData }}>
      {children}
    </PageDataContext.Provider>
  )
}

export function usePageData() {
  return useContext(PageDataContext)
}
