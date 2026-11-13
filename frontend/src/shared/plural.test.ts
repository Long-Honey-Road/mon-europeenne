import { describe, expect, it } from 'vitest'
import { plural, pluralGenitive } from './plural'

const steps = ['шаг', 'шага', 'шагов'] as const

describe('plural', () => {
  it.each([
    [1, 'шаг'],
    [2, 'шага'],
    [4, 'шага'],
    [5, 'шагов'],
    [11, 'шагов'],
    [12, 'шагов'],
    [21, 'шаг'],
    [22, 'шага'],
    [0, 'шагов'],
  ])('%i → %s', (n, form) => expect(plural(n, steps)).toBe(form))

  it.each([
    [1, 'шага'],
    [11, 'шагов'],
    [17, 'шагов'],
    [21, 'шага'],
  ])('из %i → %s', (n, form) => expect(pluralGenitive(n, ['шага', 'шагов'])).toBe(form))
})
