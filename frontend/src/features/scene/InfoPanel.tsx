import type { ItemStatus } from '../../shared/data/demo'
import type { AppSection } from '../nav/sections'

const STATUS_TEXT: Record<ItemStatus, string> = {
  done: 'Готово',
  active: 'В работе',
  waiting: 'Ждёт ответа',
  todo: 'Впереди',
}

/** Information attached to the orb in focus. */
export function InfoPanel({ section, onAsk }: { section: AppSection; onAsk: (text: string) => void }) {
  return (
    <section className="info" data-accent={section.accent} aria-labelledby="info-title">
      <p className="label">{section.stat}</p>
      <h1 id="info-title" className="info__title">
        {section.title}
      </h1>
      <p className="info__lead">{section.tagline}</p>

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

      <button type="button" className="info__ask" onClick={() => onAsk(section.prompt)}>
        Спросить buddy <span aria-hidden="true">→</span>
      </button>
    </section>
  )
}
