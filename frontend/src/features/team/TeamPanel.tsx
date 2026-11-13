import { useState } from 'react'
import type { Person } from '../../api/schemas'
import { QueryState } from '../../shared/ui/QueryState'
import { usePeople } from './usePeople'

type Scope = 'mine' | 'all'

/** Live content of the «Команда» orb: the directory with the own team first. */
export function TeamPanel({ onAsk }: { onAsk: (text: string) => void }) {
  const people = usePeople()
  const [scope, setScope] = useState<Scope>('mine')
  const [open, setOpen] = useState<string | null>(null)

  return (
    <QueryState query={people} isEmpty={(list) => list.length === 0} empty="В справочнике пока никого нет.">
      {(list) => {
        const shown = scope === 'mine' ? list.filter((p) => !p.other_team) : list
        return (
          <>
            <fieldset className="team-scope">
              <legend className="visually-hidden">Кого показать</legend>
              {(
                [
                  ['mine', 'Моя команда'],
                  ['all', 'Все'],
                ] as const
              ).map(([value, text]) => (
                <label key={value} className="team-scope__chip">
                  <input
                    type="radio"
                    name="team-scope"
                    value={value}
                    checked={scope === value}
                    onChange={() => setScope(value)}
                  />
                  {text}
                </label>
              ))}
            </fieldset>

            {shown.length === 0 ? (
              <p className="qs qs--empty">В вашей команде никого не нашли. Посмотрите всех.</p>
            ) : (
              <ul className="info__list" aria-label="Люди">
                {shown.map((p) => (
                  <PersonRow
                    key={p.id}
                    person={p}
                    expanded={open === p.id}
                    onToggle={() => setOpen(open === p.id ? null : p.id)}
                    onAsk={onAsk}
                  />
                ))}
              </ul>
            )}
          </>
        )
      }}
    </QueryState>
  )
}

function PersonRow({
  person: p,
  expanded,
  onToggle,
  onAsk,
}: {
  person: Person
  expanded: boolean
  onToggle: () => void
  onAsk: (text: string) => void
}) {
  const detailsId = `person-${p.id}`
  return (
    <li className="person">
      <button
        type="button"
        className="info__item info__item--button"
        aria-expanded={expanded}
        aria-controls={detailsId}
        onClick={onToggle}
      >
        <span className="info__text">
          <span className="info__item-title">{p.name}</span>
          <span className="info__note">{p.role_title}</span>
        </span>
        {p.other_team && <span className="info__status">другая команда</span>}
      </button>
      {expanded && (
        <div id={detailsId} className="person__details">
          {p.responsibilities.length > 0 && (
            <p>
              <span className="label">Отвечает за</span> {p.responsibilities.join(', ')}
            </p>
          )}
          {p.ask_me_about.length > 0 && (
            <p>
              <span className="label">С чем идти</span> {p.ask_me_about.join(', ')}
            </p>
          )}
          {p.contact && (
            <p>
              <span className="label">Контакт</span> {p.contact}
            </p>
          )}
          <button
            type="button"
            className="person__ask"
            aria-label={`Спросить buddy про человека: ${p.name}`}
            onClick={() => onAsk(`${p.name} — с какими вопросами идти к этому человеку?`)}
          >
            Спросить buddy <span aria-hidden="true">→</span>
          </button>
        </div>
      )}
    </li>
  )
}
