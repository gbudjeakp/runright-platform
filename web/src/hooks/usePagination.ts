import { useState, useMemo } from 'react'

export interface PaginationState {
  page: number
  pageSize: number
  search: string
}

export interface UsePaginationOptions<T> {
  items: T[]
  pageSize?: number
  searchFields?: (keyof T)[]
  searchFn?: (item: T, query: string) => boolean
}

export interface UsePaginationResult<T> {
  // Paginated & filtered items
  paginatedItems: T[]
  filteredItems: T[]
  
  // Pagination state
  page: number
  pageSize: number
  totalPages: number
  totalItems: number
  
  // Search state
  search: string
  setSearch: (s: string) => void
  
  // Navigation
  setPage: (p: number) => void
  nextPage: () => void
  prevPage: () => void
  setPageSize: (size: number) => void
  
  // Helpers
  canNextPage: boolean
  canPrevPage: boolean
  startIndex: number
  endIndex: number
}

export function usePagination<T>({
  items,
  pageSize = 10,
  searchFields = [],
  searchFn,
}: UsePaginationOptions<T>): UsePaginationResult<T> {
  const [page, setPage] = useState(1)
  const [currentPageSize, setPageSize] = useState(pageSize)
  const [search, setSearch] = useState('')

  // Filter items based on search
  const filteredItems = useMemo(() => {
    if (!search.trim()) return items
    
    const query = search.toLowerCase()
    
    return items.filter((item) => {
      if (searchFn) {
        return searchFn(item, query)
      }
      
      // Default: search in specified fields
      return searchFields.some((field) => {
        const value = item[field]
        if (typeof value === 'string') {
          return value.toLowerCase().includes(query)
        }
        if (typeof value === 'number') {
          return value.toString().includes(query)
        }
        return false
      })
    })
  }, [items, search, searchFields, searchFn])

  const totalPages = Math.max(1, Math.ceil(filteredItems.length / currentPageSize))
  
  // Reset to page 1 if current page is out of bounds
  const safePage = Math.min(page, totalPages)
  if (safePage !== page) {
    setPage(safePage)
  }

  const startIndex = (safePage - 1) * currentPageSize
  const endIndex = Math.min(startIndex + currentPageSize, filteredItems.length)

  const paginatedItems = useMemo(() => {
    return filteredItems.slice(startIndex, endIndex)
  }, [filteredItems, startIndex, endIndex])

  const canNextPage = safePage < totalPages
  const canPrevPage = safePage > 1

  const nextPage = () => {
    if (canNextPage) setPage(safePage + 1)
  }

  const prevPage = () => {
    if (canPrevPage) setPage(safePage - 1)
  }

  const handleSetSearch = (s: string) => {
    setSearch(s)
    setPage(1) // Reset to first page on search
  }

  const handleSetPageSize = (size: number) => {
    setPageSize(size)
    setPage(1) // Reset to first page on page size change
  }

  return {
    paginatedItems,
    filteredItems,
    page: safePage,
    pageSize: currentPageSize,
    totalPages,
    totalItems: filteredItems.length,
    search,
    setSearch: handleSetSearch,
    setPage,
    nextPage,
    prevPage,
    setPageSize: handleSetPageSize,
    canNextPage,
    canPrevPage,
    startIndex: startIndex + 1, // 1-indexed for display
    endIndex,
  }
}
