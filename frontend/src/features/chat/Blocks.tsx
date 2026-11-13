import type { Block, EscalationStep } from '../../api/schemas'

const roleNames: Record<string, string> = {
  mentor: 'наставник',
  manager: 'руководитель',
  hr_bp: 'HR BP',
  open_sources: 'открытые источники',
  object_owner: 'владелец ресурса',
  devops: 'DevOps',
  hr_admin: 'HR-администратор',
}

const roleName = (role: string) => roleNames[role] ?? role

export function Escalation({ steps }: { steps: EscalationStep[] }) {
  if (steps.length === 0) return null
  return (
    <ol className="block__steps" aria-label="К кому идти">
      {steps.map((s) => (
        <li key={`${s.level}-${s.role}`}>
          {s.name ? (
            <>
              <strong>{s.name}</strong> · {roleName(s.role)}
            </>
          ) : (
            roleName(s.role)
          )}
        </li>
      ))}
    </ol>
  )
}

function Sources({ block }: { block: Block }) {
  const items = block.items ?? []
  if (items.length === 0) return null
  return (
    <ul className="block block--sources" aria-label="Источники">
      {items.map((s) => (
        <li key={s.ref} className="chip">
          <span className="chip__ref">{s.ref}</span>
          {s.url ? (
            <a href={s.url} target="_blank" rel="noreferrer">
              {s.title || s.source_id}
            </a>
          ) : (
            <span>{s.title || s.source_id}</span>
          )}
        </li>
      ))}
    </ul>
  )
}

function Person({ block }: { block: Block }) {
  return (
    <section className="block block--person" aria-label={`Контакт: ${block.name ?? ''}`}>
      <span className="block__avatar" aria-hidden="true">
        {(block.name ?? '?')[0]}
      </span>
      <div>
        <p className="block__title">
          {block.name}
          {block.other_team && <span className="badge">другая команда</span>}
        </p>
        {block.role_title && <p className="block__meta">{block.role_title}</p>}
        {block.reason && <p className="block__text">{block.reason}</p>}
        {block.contact && <p className="block__contact">{block.contact}</p>}
      </div>
    </section>
  )
}

function NotFound({ block }: { block: Block }) {
  return (
    <section className="block block--warn" aria-label="Не нашёл ответа">
      <p className="block__title">Не нашёл ответа{block.topic ? `: ${block.topic}` : ''}</p>
      <p className="block__meta">Лучше спросить по цепочке:</p>
      <Escalation steps={block.escalation ?? []} />
    </section>
  )
}

type ActionApprover = { role?: string; name?: string }

// The form that edits and sends the request comes with S-05; here the prepared request is shown as is.
function Action({ block }: { block: Block }) {
  const action = block.action ?? {}
  const title = typeof action.title === 'string' ? action.title : 'Заявка'
  const fields = (action.fields && typeof action.fields === 'object' ? action.fields : {}) as Record<string, unknown>
  const approvers = (Array.isArray(action.approvers) ? action.approvers : []) as ActionApprover[]
  return (
    <section className="block block--action" aria-label={`Заявка: ${title}`}>
      <p className="block__title">{title}</p>
      <dl className="block__fields">
        {Object.entries(fields).map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{String(v)}</dd>
          </div>
        ))}
      </dl>
      {approvers.length > 0 && (
        <p className="block__meta">
          Согласует: {approvers.map((a) => a.name || roleName(a.role ?? '')).join(', ')}
        </p>
      )}
    </section>
  )
}

function Unknown({ block }: { block: Block }) {
  return (
    <section className="block" aria-label={`Блок ${block.type}`}>
      <p className="block__meta">{block.type}</p>
    </section>
  )
}

export function Blocks({ blocks }: { blocks: Block[] }) {
  if (blocks.length === 0) return null
  return (
    <div className="blocks">
      {blocks.map((b, i) => {
        const key = `${b.type}-${i}`
        switch (b.type) {
          case 'sources':
            return <Sources key={key} block={b} />
          case 'person':
            return <Person key={key} block={b} />
          case 'not_found':
            return <NotFound key={key} block={b} />
          case 'action':
            return <Action key={key} block={b} />
          default:
            return <Unknown key={key} block={b} />
        }
      })}
    </div>
  )
}
