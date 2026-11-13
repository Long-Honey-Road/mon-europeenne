import type { ReactNode } from 'react'
import type { UseQueryResult } from '@tanstack/react-query'
import './query-state.css'

/** Loading, error with retry, and empty states around data from a query. */
export function QueryState<T>({
  query,
  isEmpty,
  empty,
  children,
}: {
  query: UseQueryResult<T>
  isEmpty?: (data: T) => boolean
  empty: ReactNode
  children: (data: T) => ReactNode
}) {
  if (query.isPending) {
    return (
      <p className="qs qs--loading" role="status">
        Загружаем…
      </p>
    )
  }
  if (query.isError) {
    return (
      <div className="qs qs--error" role="alert">
        <span>Не удалось загрузить данные.</span>
        <button type="button" className="qs__retry" onClick={() => void query.refetch()}>
          Повторить
        </button>
      </div>
    )
  }
  if (isEmpty?.(query.data)) return <p className="qs qs--empty">{empty}</p>
  return <>{children(query.data)}</>
}
