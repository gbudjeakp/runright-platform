import React, { useState, useEffect, useRef, useCallback } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { sendChatMessage, fetchConversations, fetchConversation } from '../api'
import type { ChatMessage, Conversation, PageContext } from '../types'
import { usePageContext } from '../hooks/usePageContext'

// ─── Icons ──────────────────────────────────────────────────────────────────
const ChatIcon = () => (
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
  </svg>
)

const CloseIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

const SendIcon = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <line x1="22" y1="2" x2="11" y2="13" /><polygon points="22 2 15 22 11 13 2 9 22 2" />
  </svg>
)

const SparkleIcon = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
    <path d="M12 2L13.09 8.26L19 9L13.09 9.74L12 16L10.91 9.74L5 9L10.91 8.26L12 2Z" />
  </svg>
)

const MinimizeIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
    <line x1="5" y1="12" x2="19" y2="12" />
  </svg>
)

const ExpandIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="15 3 21 3 21 9" />
    <polyline points="9 21 3 21 3 15" />
    <line x1="21" y1="3" x2="14" y2="10" />
    <line x1="3" y1="21" x2="10" y2="14" />
  </svg>
)

const HistoryIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M3 3v5h5" />
    <path d="M3.05 13A9 9 0 1 0 6 5.3L3 8" />
    <path d="M12 7v5l4 2" />
  </svg>
)

const BackIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <polyline points="15 18 9 12 15 6" />
  </svg>
)

// Simple markdown renderer
const renderMarkdown = (content: string) => {
  const lines = content.split('\n')
  const elements: React.ReactElement[] = []
  let inCodeBlock = false
  let codeLines: string[] = []

  lines.forEach((line, i) => {
    if (line.startsWith('```')) {
      if (!inCodeBlock) {
        inCodeBlock = true
        codeLines = []
      } else {
        elements.push(
          <pre key={i} className="my-1.5 p-2 rounded bg-[var(--ink)]/10 overflow-x-auto text-xs">
            <code className="font-mono">{codeLines.join('\n')}</code>
          </pre>
        )
        inCodeBlock = false
      }
      return
    }
    if (inCodeBlock) {
      codeLines.push(line)
      return
    }

    // Headers
    if (line.startsWith('## ')) {
      elements.push(<h3 key={i} className="font-semibold text-sm mt-2 mb-1">{line.slice(3)}</h3>)
      return
    }
    if (line.startsWith('### ')) {
      elements.push(<h4 key={i} className="font-semibold text-xs mt-1.5 mb-0.5">{line.slice(4)}</h4>)
      return
    }

    // Bold and inline code
    let processed = line
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/`([^`]+)`/g, '<code class="px-1 py-0.5 rounded bg-[var(--ink)]/10 text-xs font-mono">$1</code>')

    // Links
    processed = processed.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" class="text-[var(--gold)] hover:underline">$1</a>')

    if (processed.trim() === '') {
      elements.push(<div key={i} className="h-2" />)
    } else if (line.startsWith('- ')) {
      elements.push(<li key={i} className="ml-3 text-sm" dangerouslySetInnerHTML={{ __html: processed.slice(2) }} />)
    } else {
      elements.push(<p key={i} className="text-sm leading-relaxed" dangerouslySetInnerHTML={{ __html: processed }} />)
    }
  })

  return elements
}

export default function ChatWidget() {
  const location = useLocation()
  const [isOpen, setIsOpen] = useState(false)
  const [isExpanded, setIsExpanded] = useState(false)
  const [showHistory, setShowHistory] = useState(false)
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [conversationId, setConversationId] = useState<string | null>(null)

  const navigate = useNavigate()
  const pageContext = usePageContext()
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // Handle clicks on internal links in assistant messages
  const handleMessageClick = useCallback((e: React.MouseEvent) => {
    const target = e.target as HTMLElement
    if (target.tagName === 'A') {
      const href = target.getAttribute('href')
      if (href?.startsWith('/app/')) {
        e.preventDefault()
        navigate(href)
        setIsOpen(false) // Close chat widget after navigation
      }
    }
  }, [navigate])

  // Check if on assistant page (used later for conditional render)
  const isAssistantPage = location.pathname === '/app/assistant'

  // Load conversations when widget opens
  useEffect(() => {
    if (isOpen) {
      fetchConversations().then(setConversations).catch(console.error)
    }
  }, [isOpen])

  // Scroll to bottom on new messages
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // Focus input when opened
  useEffect(() => {
    if (isOpen && !showHistory) {
      setTimeout(() => inputRef.current?.focus(), 100)
    }
  }, [isOpen, showHistory])

  const loadConversation = async (convId: string) => {
    try {
      const data = await fetchConversation(convId)
      setConversationId(convId)
      setMessages(data.messages)
      setShowHistory(false)
    } catch (e) {
      console.error('Failed to load conversation:', e)
    }
  }

  const handleSend = useCallback(async () => {
    const text = input.trim()
    if (!text || loading) return

    setInput('')
    setLoading(true)

    const userMsg: ChatMessage = {
      id: `temp-${Date.now()}`,
      conversation_id: conversationId ?? '',
      role: 'user',
      content: text,
      created_at: new Date().toISOString(),
    }
    setMessages(prev => [...prev, userMsg])

    try {
      const response = await sendChatMessage({
        conversation_id: conversationId ?? undefined,
        message: text,
        page_context: pageContext,
      })

      if (!conversationId) {
        setConversationId(response.conversation_id)
        // Refresh conversations list
        fetchConversations().then(setConversations).catch(console.error)
      }

      setMessages(prev => {
        const updated = prev.map(m =>
          m.id === userMsg.id ? { ...m, id: `user-${Date.now()}`, conversation_id: response.conversation_id } : m
        )
        return [...updated, response.message]
      })
    } catch (e: unknown) {
      const errorMsg = e instanceof Error ? e.message : 'Failed to send message'
      setMessages(prev => [
        ...prev,
        {
          id: `error-${Date.now()}`,
          conversation_id: conversationId ?? '',
          role: 'assistant',
          content: `Error: ${errorMsg}`,
          created_at: new Date().toISOString(),
        },
      ])
    } finally {
      setLoading(false)
    }
  }, [input, loading, conversationId, pageContext])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const startNewChat = () => {
    setMessages([])
    setConversationId(null)
    setShowHistory(false)
  }

  // Page context indicator - format nicely
  const formatPage = (page: string) => 
    page.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
  
  const pageLabel = pageContext.entity_type 
    ? `${formatPage(pageContext.entity_type)} ${pageContext.entity_id}`
    : formatPage(pageContext.page)

  // Format date for conversation list
  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr)
    const now = new Date()
    const diffDays = Math.floor((now.getTime() - date.getTime()) / (1000 * 60 * 60 * 24))
    if (diffDays === 0) return 'Today'
    if (diffDays === 1) return 'Yesterday'
    if (diffDays < 7) return `${diffDays}d ago`
    return date.toLocaleDateString()
  }

  // Don't render on the assistant page (must be after all hooks)
  if (isAssistantPage) {
    return null
  }

  if (!isOpen) {
    return (
      <button
        onClick={() => setIsOpen(true)}
        className="fixed bottom-5 right-5 z-50 flex items-center justify-center w-14 h-14 rounded-full bg-[var(--gold)] text-[var(--ink)] shadow-lg hover:scale-105 transition-transform"
        aria-label="Open AI Assistant"
      >
        <ChatIcon />
      </button>
    )
  }

  return (
    <div
      className={[
        'fixed z-50 flex flex-col rounded-xl shadow-2xl border overflow-hidden transition-all duration-200',
        'bg-[#faf8f3] dark:bg-[#1a1a1a] border-[var(--border)] dark:border-[#333]',
        'text-[var(--text)] dark:text-[#e5e5e5]',
        isExpanded
          ? 'bottom-5 right-5 w-[600px] h-[80vh] max-h-[800px]'
          : 'bottom-5 right-5 w-[380px] h-[500px]',
      ].join(' ')}
    >
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)] dark:border-[#333] bg-[var(--gold)]/10 dark:bg-[var(--gold)]/20">
        <div className="flex items-center gap-2 text-[var(--text)] dark:text-[#e5e5e5]">
          {showHistory && (
            <button
              onClick={() => setShowHistory(false)}
              className="p-1 rounded hover:bg-black/10 dark:hover:bg-white/10"
              title="Back to chat"
            >
              <BackIcon />
            </button>
          )}
          <SparkleIcon />
          <span className="font-semibold text-sm">{showHistory ? 'History' : 'RunRight AI'}</span>
        </div>
        <div className="flex items-center gap-1">
          {!showHistory && (
            <>
              <button
                onClick={() => setShowHistory(true)}
                className="p-1.5 rounded hover:bg-black/10 dark:hover:bg-white/10 text-[var(--text-mid)] dark:text-[#999]"
                title="Conversation history"
              >
                <HistoryIcon />
              </button>
              <button
                onClick={startNewChat}
                className="p-1.5 rounded hover:bg-black/10 dark:hover:bg-white/10 text-[var(--text-mid)] dark:text-[#999] text-xs"
                title="New chat"
              >
                New
              </button>
            </>
          )}
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="p-1.5 rounded hover:bg-black/10 dark:hover:bg-white/10 text-[var(--text-mid)] dark:text-[#999]"
            title={isExpanded ? 'Minimize' : 'Expand'}
          >
            {isExpanded ? <MinimizeIcon /> : <ExpandIcon />}
          </button>
          <button
            onClick={() => setIsOpen(false)}
            className="p-1.5 rounded hover:bg-black/10 dark:hover:bg-white/10 text-[var(--text-mid)] dark:text-[#999]"
            title="Close"
          >
            <CloseIcon />
          </button>
        </div>
      </div>

      {showHistory ? (
        /* Conversation History List */
        <div className="flex-1 overflow-y-auto">
          <button
            onClick={startNewChat}
            className="w-full px-4 py-3 text-left border-b border-[var(--border)] dark:border-[#333] hover:bg-[var(--gold)]/10 transition-colors"
          >
            <div className="flex items-center gap-2 text-[var(--gold)]">
              <SparkleIcon />
              <span className="text-sm font-medium">New conversation</span>
            </div>
          </button>
          {conversations.length === 0 ? (
            <div className="p-4 text-center text-[var(--text-mid)] dark:text-[#888] text-sm">
              No conversations yet
            </div>
          ) : (
            conversations.map((conv) => (
              <button
                key={conv.id}
                onClick={() => loadConversation(conv.id)}
                className={[
                  'w-full px-4 py-3 text-left border-b border-[var(--border)] dark:border-[#333] hover:bg-black/5 dark:hover:bg-white/5 transition-colors',
                  conv.id === conversationId ? 'bg-[var(--gold)]/10' : '',
                ].join(' ')}
              >
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium truncate flex-1">{conv.title || 'Untitled'}</span>
                  <span className="text-xs text-[var(--text-mid)] dark:text-[#888] ml-2">{formatDate(conv.updated_at)}</span>
                </div>
                <div className="text-xs text-[var(--text-mid)] dark:text-[#888] mt-0.5">
                  {conv.message_count} message{conv.message_count !== 1 ? 's' : ''}
                </div>
              </button>
            ))
          )}
        </div>
      ) : (
        /* Chat View */
        <>
          {/* Context indicator */}
          <div className="px-3 py-1.5 bg-black/5 dark:bg-white/5 border-b border-[var(--border)] dark:border-[#333] text-xs text-[var(--text-mid)] dark:text-[#888]">
            On: <span className="font-medium text-[var(--text)] dark:text-[#e5e5e5]">{pageLabel}</span>
          </div>

          {/* Messages */}
          <div className="flex-1 overflow-y-auto px-3 py-3 space-y-3">
            {messages.length === 0 && (
              <div className="flex flex-col items-center justify-center h-full text-center text-[var(--text-mid)] dark:text-[#888]">
                <SparkleIcon />
                <p className="mt-2 text-sm font-medium">Ask me anything</p>
                <p className="text-xs mt-1">I can help with jobs, alerts, policies, and more</p>
              </div>
            )}
            {messages.map((msg) => (
              <div
                key={msg.id}
                className={[
                  'max-w-[85%] px-3 py-2 rounded-lg',
                  msg.role === 'user'
                    ? 'ml-auto bg-[var(--gold)] text-[#1a1a1a]'
                    : 'bg-black/5 dark:bg-white/10 text-[var(--text)] dark:text-[#e5e5e5]',
                ].join(' ')}
                onClick={msg.role === 'assistant' ? handleMessageClick : undefined}
              >
                {msg.role === 'user' ? (
                  <p className="text-sm">{msg.content}</p>
                ) : (
                  <div className="prose-sm">{renderMarkdown(msg.content)}</div>
                )}
              </div>
            ))}
            {loading && (
              <div className="flex items-center gap-2 text-[var(--text-mid)] dark:text-[#888] text-sm">
                <div className="flex gap-1">
                  <span className="w-1.5 h-1.5 bg-[var(--gold)] rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                  <span className="w-1.5 h-1.5 bg-[var(--gold)] rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                  <span className="w-1.5 h-1.5 bg-[var(--gold)] rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
                </div>
                Thinking...
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          {/* Input */}
          <div className="p-3 border-t border-[var(--border)] dark:border-[#333]">
            <div className="flex items-end gap-2">
              <textarea
                ref={inputRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Ask about this page..."
                className="flex-1 resize-none rounded-lg border border-[var(--border)] dark:border-[#444] bg-transparent dark:bg-[#222] px-3 py-2 text-sm text-[var(--text)] dark:text-[#e5e5e5] placeholder:text-[var(--text-mid)] dark:placeholder:text-[#666] focus:outline-none focus:ring-2 focus:ring-[var(--gold)]/50 min-h-[40px] max-h-[100px]"
                rows={1}
              />
              <button
                onClick={handleSend}
                disabled={!input.trim() || loading}
                className="flex items-center justify-center w-9 h-9 rounded-lg bg-[var(--gold)] text-[#1a1a1a] disabled:opacity-50 disabled:cursor-not-allowed hover:brightness-110 transition-all"
              >
                <SendIcon />
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
