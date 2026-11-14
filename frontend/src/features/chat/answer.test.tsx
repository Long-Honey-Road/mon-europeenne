import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderApp } from '../../test/render'
import { addMessage, addThread, richAnswer } from '../../test/chat'

// Visible text: KaTeX keeps the TeX source in a MathML annotation for screen readers, which is not shown.
function visibleText(el: HTMLElement): string {
  const copy = el.cloneNode(true) as HTMLElement
  copy.querySelectorAll('annotation').forEach((a) => a.remove())
  return copy.textContent ?? ''
}

async function renderAnswer(text: string) {
  const t = addThread()
  addMessage(t.id, 'assistant', text)
  renderApp('/')
  const log = within(await screen.findByRole('complementary', { name: 'Чат с buddy' }))
  const item = await log.findByText((_, el) => el?.classList.contains('msg__text') ?? false)
  return item as HTMLElement
}

describe('answer formatting', () => {
  it('renders math, a table, highlighted code, task lists and strikethrough', async () => {
    const answer = await renderAnswer(richAnswer)

    expect(answer.querySelectorAll('.katex').length).toBeGreaterThanOrEqual(2)
    expect(answer.querySelector('.katex-display .mfrac')).not.toBeNull()
    const text = visibleText(answer)
    expect(text).not.toContain('$')
    expect(text).not.toContain('\\frac')

    const table = within(answer).getByRole('table')
    expect(within(table).getByRole('columnheader', { name: 'Метрика' })).toBeInTheDocument()
    expect(within(table).getByRole('cell', { name: '200 мс' })).toBeInTheDocument()
    expect(table.parentElement).toHaveClass('msg__table')

    const code = answer.querySelector('pre code.hljs.language-go')
    expect(code).not.toBeNull()
    expect(code?.querySelector('.hljs-keyword')).toHaveTextContent('func')

    expect(within(answer).getByText('Старый способ').tagName).toBe('DEL')
    expect(within(answer).getAllByRole('checkbox')).toHaveLength(2)
  })

  it('keeps $$ inside a sentence inline', async () => {
    const answer = await renderAnswer('Доля $$\\frac{a}{b}$$ от всех.')

    expect(answer.querySelector('.katex .mfrac')).not.toBeNull()
    expect(answer.querySelector('.katex-display')).toBeNull()
  })

  it('keeps inline code and citation markers as they were', async () => {
    const answer = await renderAnswer(richAnswer)

    const inline = within(answer).getByText('make deploy')
    expect(inline.tagName).toBe('CODE')
    expect(inline).not.toHaveClass('hljs')
    expect(answer).toHaveTextContent('подробности в [1].')
    expect(within(answer).queryByRole('link')).not.toBeInTheDocument()
  })

  it('does not render raw HTML from the model', async () => {
    const answer = await renderAnswer('Текст <img src="x" onerror="alert(1)"> и <b>жирный</b>.')

    expect(answer.querySelector('img')).toBeNull()
    expect(answer.querySelector('b')).toBeNull()
  })

  it('leaves code in an unknown language as plain text', async () => {
    const answer = await renderAnswer('```brainfuck\n++[>+<-]\n```')

    expect(answer.querySelector('pre code')).toHaveTextContent('++[>+<-]')
    expect(answer.querySelector('pre code [class^="hljs-"]')).toBeNull()
  })
})
