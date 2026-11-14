import type { Block, EscalationStep } from '../../api/schemas'
import { RequestAction } from '../requests/RequestAction'
import { roleName } from '../requests/roles'

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

type SourceItem = NonNullable<Block['items']>[number]

// Sources that are not the team wiki are marked, so the intern sees where a fact comes from.
const kindBadges: Record<string, string> = {
  playbook: 'открытый Playbook',
  checklist: 'чек-лист Авито',
  guide: 'о buddy',
}

function SourceLink({ source }: { source: SourceItem }) {
  const label = source.title || source.source_id
  return (
    <>
      {source.url ? (
        <a href={source.url} target="_blank" rel="noreferrer">
          {label}
        </a>
      ) : (
        <span>{label}</span>
      )}
      {source.kind && kindBadges[source.kind] && <span className="badge">{kindBadges[source.kind]}</span>}
    </>
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
          <SourceLink source={s} />
        </li>
      ))}
    </ul>
  )
}

// Two documents of one topic: a general rule and the team's own. The team rule applies to the team.
function Conflict({ block }: { block: Block }) {
  const [general, team] = block.items ?? []
  if (!general || !team) return null
  return (
    <section className="block block--caution" aria-label="Документы расходятся">
      <p className="block__title">Документы расходятся{block.topic ? `: ${block.topic}` : ''}</p>
      <ul className="block__pair">
        <li>
          <span className="block__meta">Общее правило</span> <SourceLink source={general} />
        </li>
        <li>
          <span className="block__meta">Правило команды</span> <SourceLink source={team} />
        </li>
      </ul>
      <p className="block__meta">Для твоей команды действует правило команды. Если сомневаешься, уточни у наставника.</p>
    </section>
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

// The server adds this block whenever the answer used the model's own knowledge instead of company data.
function GeneralKnowledge({ block }: { block: Block }) {
  return (
    <section className="block block--caution" aria-label="Общие знания">
      <p className="block__title">Это общие знания модели{block.topic ? `: ${block.topic}` : ''}</p>
      <p className="block__meta">
        В документах Авито этого нет. Ответ может быть неточным или устаревшим: проверь у наставника, прежде чем
        делать.
      </p>
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
            return <RequestAction key={key} action={b.action} />
          case 'general_knowledge':
            return <GeneralKnowledge key={key} block={b} />
          case 'conflict':
            return <Conflict key={key} block={b} />
          default:
            return <Unknown key={key} block={b} />
        }
      })}
    </div>
  )
}
