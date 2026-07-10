import { useLocation } from 'react-router-dom'
import type { PageContext } from '../types'
import { usePageData } from '../contexts/PageDataContext'

/**
 * Hook to get the current page context for the assistant.
 * Extracts page name, entity type, and entity ID from the current route.
 * Also includes any page-specific data provided by the current page.
 * 
 * Note: We parse params from pathname directly because useParams() only works
 * inside route components, and ChatWidget is rendered at the AppShell level.
 */
export function usePageContext(): PageContext {
  const location = useLocation()
  const { pageData } = usePageData()
  const path = location.pathname

  // Parse job detail: /app/jobs/:id or /app/jobs/group/:jobId
  const jobMatch = path.match(/\/app\/jobs\/(?:group\/)?([^/]+)$/)
  if (jobMatch) {
    return {
      page: 'job_detail',
      entity_type: 'job',
      entity_id: decodeURIComponent(jobMatch[1]),
      metadata: { path },
      page_data: pageData,
    }
  }

  // Parse repo detail: /app/repos/:repoId  
  const repoMatch = path.match(/\/app\/repos\/([^/]+)$/)
  if (repoMatch) {
    return {
      page: 'repo_detail',
      entity_type: 'repository',
      entity_id: decodeURIComponent(repoMatch[1]),
      metadata: { path },
      page_data: pageData,
    }
  }

  // Parse run detail: /app/runs/:runId
  const runMatch = path.match(/\/app\/runs\/([^/]+)$/)
  if (runMatch) {
    return {
      page: 'run_detail',
      entity_type: 'run',
      entity_id: decodeURIComponent(runMatch[1]),
      metadata: { path },
      page_data: pageData,
    }
  }

  // Top-level pages mapping
  const pageMap: Record<string, string> = {
    '/app': 'jobs',
    '/app/jobs': 'jobs',
    '/app/repos': 'repos',
    '/app/alerts': 'alerts',
    '/app/policies': 'policies',
    '/app/settings': 'settings',
    '/app/analytics': 'analytics',
    '/app/assistant': 'assistant',
    '/app/catalog': 'catalog',
    '/app/history': 'activity',
    '/app/auto-pr': 'auto_pr',
  }

  const pageName = pageMap[path]
  if (pageName) {
    return {
      page: pageName,
      metadata: { path },
      page_data: pageData,
    }
  }

  // Fallback: extract page name from path
  return {
    page: path.replace('/app/', '').replace(/\//g, '_') || 'unknown',
    metadata: { path },
    page_data: pageData,
  }
}
