import { useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { ChatPanel } from '../chat/ChatPanel'
import { sectionByPath } from '../nav/sections'
import { Scene } from '../scene/Scene'
import './home.css'

/** The single screen: orb scene on the left, buddy chat on the right. */
export function Home() {
  const pathname = useLocation({ select: (l) => l.pathname })
  const navigate = useNavigate()
  const active = sectionByPath(pathname)
  const [draft, setDraft] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!active) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') void navigate({ to: '/' })
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [active, navigate])

  const ask = (text: string) => {
    setDraft(text)
    inputRef.current?.focus()
  }

  return (
    <div className="home">
      <Scene active={active} onAsk={ask} />
      <ChatPanel context={active} draft={draft} onDraftChange={setDraft} inputRef={inputRef} />
    </div>
  )
}
