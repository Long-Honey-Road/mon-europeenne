import { planItems, workItems } from '../../shared/data/demo'
import type { InfoItem } from '../../shared/data/demo'

export type Accent = 'green' | 'blue' | 'red' | 'purple'

export type Section = {
  id: string
  path: string
  accent: Accent
  /** Hex of the accent, for canvas drawing. */
  color: string
  /** Short name shown on the orb. */
  name: string
  title: string
  tagline: string
  /** Demo list for sections without live data yet; live sections render their own panel. */
  items: readonly InfoItem[]
  /** Question the "ask buddy" button puts into the chat input. */
  prompt: string
  /** Orb centre and diameter in % of the logo box, taken from the Avito logo. */
  orb: { x: number; y: number; d: number }
}

export const sections = [
  {
    id: 'today',
    path: '/today',
    accent: 'green',
    color: '#04e061',
    name: 'Сегодня',
    title: 'Сегодня',
    tagline: 'Ближайшие шаги и то, что ждёт чужого ответа.',
    items: [],
    prompt: 'Что мне сделать сегодня в первую очередь?',
    orb: { x: 32.4, y: 64.4, d: 52.7 },
  },
  {
    id: 'plan',
    path: '/plan',
    accent: 'blue',
    color: '#00aaff',
    name: 'План',
    title: 'План адаптации',
    tagline: 'Этапы первых недель и прогресс по каждому.',
    items: planItems,
    prompt: 'Какой следующий шаг в моём плане адаптации?',
    orb: { x: 71.9, y: 30.5, d: 42.9 },
  },
  {
    id: 'work',
    path: '/work',
    accent: 'red',
    color: '#ff4053',
    name: 'Задачи',
    title: 'Задачи и заявки',
    tagline: 'Рабочие задачи и запросы через корпоративные сервисы.',
    items: workItems,
    prompt: 'В каком статусе мои заявки на доступы?',
    orb: { x: 78.3, y: 71.2, d: 31.7 },
  },
  {
    id: 'team',
    path: '/team',
    accent: 'purple',
    color: '#965eeb',
    name: 'Команда',
    title: 'Команда',
    tagline: 'Кто за что отвечает и с каким вопросом к кому идти.',
    items: [],
    prompt: 'К кому идти, если не собирается проект?',
    orb: { x: 37.3, y: 24.4, d: 21 },
  },
] as const satisfies readonly Section[]

export type AppSection = (typeof sections)[number]

export function sectionByPath(pathname: string): AppSection | undefined {
  return sections.find((s) => s.path === pathname)
}
