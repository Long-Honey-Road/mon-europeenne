import { createContext, useContext } from 'react'

/** The chat thread whose messages are on screen; a sent request is linked to it. */
export const ChatThreadContext = createContext<string | null>(null)

export const useChatThread = () => useContext(ChatThreadContext)
