import type { AppSection } from '../nav/sections'
import { PlanPanel } from '../plan/PlanPanel'
import { WorkPanel } from '../task/WorkPanel'
import { TeamPanel } from '../team/TeamPanel'
import { TodayPanel } from '../today/TodayPanel'

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
        <WorkPanel />
      )}

      <button type="button" className="info__ask" onClick={() => onAsk(section.prompt)}>
        Спросить buddy <span aria-hidden="true">→</span>
      </button>
    </section>
  )
}
