import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useSession } from './session'

/** Sign-out for the scene header. */
export function UserMenu() {
  const signOut = useSession((s) => s.signOut)
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const exit = () => {
    signOut()
    queryClient.clear()
    void navigate({ to: '/login' })
  }

  return (
    <button type="button" className="user-menu__exit" onClick={exit}>
      Выйти
    </button>
  )
}
