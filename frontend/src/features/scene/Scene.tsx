import type { CSSProperties } from 'react'
import { Link } from '@tanstack/react-router'
import { intern } from '../../shared/data/demo'
import { UserMenu } from '../auth/UserMenu'
import { useCurrentUser } from '../auth/useMe'
import { sections } from '../nav/sections'
import type { AppSection } from '../nav/sections'
import { InfoPanel } from './InfoPanel'
import { SceneCanvas } from './SceneCanvas'
import './scene.css'

/** The orb scene. Without `active` the orbs form the logo; with it one orb takes the stage. */
export function Scene({ active, onAsk }: { active?: AppSection; onAsk: (text: string) => void }) {
  const minis = sections.filter((s) => s !== active)
  const firstName = useCurrentUser()?.firstName

  return (
    <section className="scene" data-accent={active?.accent} aria-label="Сцена разделов">
      <header className="scene__top">
        <Link to="/" className="scene__brand" aria-label="AI-buddy, все разделы">
          <span className="logo" aria-hidden="true">
            {sections.map((s) => (
              <i
                key={s.id}
                data-accent={s.accent}
                style={{ left: `${s.orb.x}%`, top: `${s.orb.y}%`, width: `${s.orb.d}%` }}
              />
            ))}
          </span>
          AI-buddy
        </Link>
        <div className="scene__actions">
          {active && (
            <Link to="/" className="scene__back">
              <span aria-hidden="true">←</span> Все разделы
            </Link>
          )}
          <UserMenu />
        </div>
      </header>

      <nav className="scene__orbs" aria-label="Разделы">
        {sections.map((s) => {
          const state = !active ? 'cluster' : s === active ? 'main' : 'mini'
          const style = {
            '--ox': s.orb.x - 50,
            '--oy': s.orb.y - 50,
            '--d': s.orb.d,
            '--k': minis.indexOf(s),
          } as CSSProperties
          return (
            <Link
              key={s.id}
              to={s.path}
              className="orb"
              data-accent={s.accent}
              data-state={state}
              style={style}
              aria-current={s === active ? 'page' : undefined}
            >
              <span className="orb__scale">
                <span className="orb__glow" />
                <span className="orb__ball" data-orb-ball data-state={state} data-color={s.color} />
              </span>
              <span className="orb__label">
                <span className="orb__name">{s.name}</span>
                <span className="orb__stat">{s.stat}</span>
              </span>
            </Link>
          )
        })}
      </nav>

      {/* Above the orbs so near-side particles pass in front of them. */}
      <SceneCanvas />

      {active ? (
        <InfoPanel key={active.id} section={active} onAsk={onAsk} />
      ) : (
        <div className="scene__hello">
          <p className="label">
            {intern.team} · день {intern.day} из {intern.totalDays}
          </p>
          <h1 className="scene__title">{firstName ? `Привет, ${firstName}` : 'Привет'}</h1>
          <p className="scene__lead">Выбери сферу — покажу, что внутри.</p>
        </div>
      )}

      <p className="scene__foot label">Учебный прототип · данные синтетические</p>
    </section>
  )
}
