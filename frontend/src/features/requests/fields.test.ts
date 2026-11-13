import { describe, expect, it } from 'vitest'
import { fieldsFromSchema, formSchema, initialValues, payload } from './fields'

const schema = {
  type: 'object',
  required: ['level', 'justification'],
  properties: {
    level: { enum: ['read', 'write'], title: 'Уровень' },
    justification: { type: 'string', title: 'Обоснование', minLength: 10 },
    env: { enum: ['dev', 'prod'] },
    note: { type: 'string' },
  },
}

describe('request fields from JSON Schema', () => {
  const fields = fieldsFromSchema(schema)

  it('reads kinds, labels and required flags', () => {
    expect(fields.map((f) => [f.key, f.kind, f.label, f.required])).toEqual([
      ['level', 'choice', 'Уровень', true],
      ['justification', 'long_text', 'Обоснование', true],
      ['env', 'choice', 'env', false],
      ['note', 'text', 'note', false],
    ])
  })

  it('accepts empty optional fields and rejects bad values', () => {
    const check = formSchema(fields)
    expect(check.safeParse({ level: 'read', justification: 'достаточно длинно', env: '', note: '' }).success).toBe(true)
    const bad = check.safeParse({ level: 'admin', justification: '', env: 'stage', note: '' })
    expect(bad.success).toBe(false)
    const messages = Object.fromEntries((bad.error?.issues ?? []).map((i) => [String(i.path[0]), i.message]))
    expect(messages).toEqual({
      level: 'выбери вариант из списка',
      justification: 'обязательное поле',
      env: 'выбери вариант из списка',
    })
  })

  it('fills values from the prepared request and drops empty optional fields', () => {
    const values = initialValues(fields, { level: 'read', justification: 42, note: null })
    expect(values).toEqual({ level: 'read', justification: '42', env: '', note: '' })
    expect(payload(fields, { ...values, justification: '  текст  ' })).toEqual({ level: 'read', justification: 'текст' })
  })

  it('treats a schema without properties as no fields', () => {
    expect(fieldsFromSchema({ type: 'object', properties: [] })).toEqual([])
  })
})
