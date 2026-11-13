import { z } from 'zod'

/** One form field read from a request type's `fields_schema` (the JSON Schema subset the catalog uses). */
export type Field = {
  key: string
  label: string
  required: boolean
  kind: 'choice' | 'text' | 'long_text'
  options: string[]
  minLength: number
}

const asRecord = (v: unknown): Record<string, unknown> =>
  v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {}

export function fieldsFromSchema(schema: Record<string, unknown>): Field[] {
  const required = new Set(Array.isArray(schema.required) ? schema.required.map(String) : [])
  return Object.entries(asRecord(schema.properties)).map(([key, raw]) => {
    const p = asRecord(raw)
    const options = Array.isArray(p.enum) ? p.enum.map(String) : []
    const minLength = typeof p.minLength === 'number' ? p.minLength : 0
    return {
      key,
      label: typeof p.title === 'string' && p.title ? p.title : key,
      required: required.has(key),
      kind: options.length > 0 ? 'choice' : minLength >= 10 ? 'long_text' : 'text',
      options,
      minLength,
    }
  })
}

/** Zod check matching the server's: required fields, allowed choices, minimal length. */
export function formSchema(fields: Field[]) {
  const shape: Record<string, z.ZodType<string>> = {}
  for (const f of fields) {
    if (f.kind === 'choice') {
      shape[f.key] = z
        .string()
        .refine((v) => (!f.required && v === '') || f.options.includes(v), 'выбери вариант из списка')
      continue
    }
    const text = z.string().trim()
    const base = f.required ? text.min(1, 'обязательное поле') : text
    shape[f.key] =
      f.minLength > 0
        ? base.refine((v) => v === '' || v.length >= f.minLength, `не короче ${f.minLength} символов`)
        : base
  }
  return z.object(shape)
}

/** Form values from the prepared request; everything is a string in the inputs. */
export function initialValues(fields: Field[], prepared: Record<string, unknown>): Record<string, string> {
  return Object.fromEntries(fields.map((f) => [f.key, prepared[f.key] == null ? '' : String(prepared[f.key])]))
}

/** What goes to the service: trimmed, without empty optional fields. */
export function payload(fields: Field[], values: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const f of fields) {
    const v = (values[f.key] ?? '').trim()
    if (v !== '' || f.required) out[f.key] = v
  }
  return out
}
