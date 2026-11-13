import type { ItemStatus } from '../../shared/data/demo'
import type { AppSection } from '../nav/sections'
import { PlanPanel } from '../plan/PlanPanel'
import { TeamPanel } from '../team/TeamPanel'
import { TodayPanel } from '../today/TodayPanel'

const STATUS_TEXT: Record<ItemStatus, string> = {
  done: 'Готово',
  active: 'В работе',
  waiting: 'Ждёт ответа',
  todo: 'Впереди',
}

/** Information attached to the orb in focus. */
export function InfoPanel({
  section,
  stat,
  onAsk,
}: {
  section: AppSection
  stat: string
  onAsk: (text: string) => void
}) {
  return (
    <section className="info" data-accent={section.accent} aria-labelledby="info-title">
      {stat && <p className="label">{stat}</p>}
      <h1 id="info-title" className="info__title">
        {section.title}
      </h1>
      <p className="info__lead">{section.tagline}</p>

      {section.id === 'today' ? (
        <TodayPanel onAsk={onAsk} />
      ) : section.id === 'team' ? (
        <TeamPanel onAsk={onAsk} />
      ) : section.id === 'plan' ? (
        <PlanPanel onAsk={onAsk} />
      ) : (
        <DemoList section={section} />
      )}

      <button type="button" className="info__ask" onClick={() => onAsk(section.prompt)}>
        Спросить buddy <span aria-hidden="true">→</span>
      </button>
    </section>
  )
}

// Tasks stay on demo data until S-08.
function DemoList({ section }: { section: AppSection }) {
  return (
    <ul className="info__list">
      {section.items.map((item) => (
        <li key={item.title} className="info__item">
          <span className="info__text">
            <span className="info__item-title">{item.title}</span>
            <span className="info__note">{item.note}</span>
          </span>
          {item.status && (
            <span className={`info__status info__status--${item.status}`}>{STATUS_TEXT[item.status]}</span>
          )}
        </li>
      ))}
    </ul>
  )
}
