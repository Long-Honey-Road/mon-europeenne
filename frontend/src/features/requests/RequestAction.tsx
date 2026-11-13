import { useMemo, useRef, useState } from 'react'
import type { FormEvent, MutableRefObject } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { api, ApiError } from '../../api'
import { approverSchema } from '../../api/schemas'
import type { Approver, RequestStatus, ServiceRequest } from '../../api/schemas'
import { QueryState } from '../../shared/ui/QueryState'
import { fieldsFromSchema, formSchema, initialValues, payload } from './fields'
import type { Field } from './fields'
import { roleName } from './roles'
import { useChatThread } from './thread'
import { rememberRequest, useMyRequests, useRequestTypes } from './useRequests'

const actionSchema = z.object({
  type_key: z.string().min(1),
  title: z.string().catch('Заявка'),
  fields: z.record(z.string(), z.unknown()).catch({}),
  approvers: z.array(approverSchema).catch([]),
})

type PreparedAction = z.infer<typeof actionSchema>

const approverNames = (list: Approver[]) => list.map((a) => a.name || roleName(a.role)).join(', ')

/** The `action` block: a request prepared by buddy that the user checks, edits and sends, then its status. */
export function RequestAction({ action }: { action: Record<string, unknown> | undefined }) {
  const threadId = useChatThread()
  const mine = useMyRequests()
  const parsed = actionSchema.safeParse(action ?? {})
  if (!parsed.success) return null
  const prepared = parsed.data
  const sent = mine.data?.find((r) => r.thread_id === threadId && r.type_key === prepared.type_key)
  if (sent) return <RequestStatusCard request={sent} />
  return <RequestForm prepared={prepared} threadId={threadId} />
}

function RequestForm({ prepared, threadId }: { prepared: PreparedAction; threadId: string | null }) {
  const types = useRequestTypes()
  const sendRef = useRef<((e: FormEvent) => void) | null>(null)
  const title = `Заявка: ${prepared.title}`
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    sendRef.current?.(e)
  }
  return (
    <form className="block block--action request" aria-label={title} onSubmit={onSubmit} noValidate>
      <p className="block__title">{prepared.title}</p>
      <QueryState query={types} empty={null}>
        {(list) => {
          const type = list.find((t) => t.key === prepared.type_key)
          if (!type) return <p className="block__meta">Такого типа заявки нет в каталоге: спроси buddy ещё раз.</p>
          return (
            <RequestFields
              key={type.key}
              prepared={prepared}
              schema={type.fields_schema}
              threadId={threadId}
              sendRef={sendRef}
            />
          )
        }}
      </QueryState>
    </form>
  )
}

function submitError(error: unknown): string {
  if (error instanceof ApiError && error.kind === 'server' && error.status === 400) {
    return `Сервис заявок не принял: ${error.message}`
  }
  if (error instanceof ApiError && (error.kind === 'network' || error.status >= 502)) {
    return 'Сервис заявок не отвечает. Попробуй отправить ещё раз чуть позже.'
  }
  return 'Не получилось отправить заявку. Попробуй ещё раз.'
}

function RequestFields({
  prepared,
  schema,
  threadId,
  sendRef,
}: {
  prepared: PreparedAction
  schema: Record<string, unknown>
  threadId: string | null
  /** The form element lives above; it calls this on submit, by button or Enter. */
  sendRef: MutableRefObject<((e: FormEvent) => void) | null>
}) {
  const fields = useMemo(() => fieldsFromSchema(schema), [schema])
  const check = useMemo(() => formSchema(fields), [fields])
  const [values, setValues] = useState(() => initialValues(fields, prepared.fields))
  const [problems, setProblems] = useState<Record<string, string>>({})
  const qc = useQueryClient()

  const submit = useMutation({
    mutationFn: (fieldsToSend: Record<string, string>) =>
      api.submitRequest(threadId ?? '', prepared.type_key, fieldsToSend),
    onSuccess: (r) => rememberRequest(qc, r),
  })

  const send = () => {
    const result = check.safeParse(values)
    if (!result.success) {
      const next: Record<string, string> = {}
      for (const issue of result.error.issues) {
        const key = String(issue.path[0])
        next[key] ??= issue.message
      }
      setProblems(next)
      return
    }
    setProblems({})
    submit.mutate(payload(fields, values))
  }
  sendRef.current = send

  const label = (key: string) => fields.find((f) => f.key === key)?.label ?? key
  const errors = Object.entries(problems)

  return (
    <div className="request__body">
      {fields.map((f) => (
        <FieldInput
          key={f.key}
          field={f}
          value={values[f.key] ?? ''}
          invalid={f.key in problems}
          disabled={submit.isPending}
          onChange={(v) => setValues((prev) => ({ ...prev, [f.key]: v }))}
        />
      ))}

      {prepared.approvers.length > 0 && (
        <p className="block__meta">Согласует: {approverNames(prepared.approvers)}</p>
      )}

      {errors.length > 0 && (
        <div className="request__error" role="alert">
          {errors.map(([k, msg]) => (
            <p key={k}>
              {label(k)}: {msg}
            </p>
          ))}
        </div>
      )}
      {submit.isError && errors.length === 0 && (
        <p className="request__error" role="alert">
          {submitError(submit.error)}
        </p>
      )}

      <button type="submit" className="request__send" disabled={submit.isPending || !threadId}>
        {submit.isPending ? 'Отправляем…' : 'Отправить заявку'}
      </button>
      <p className="request__hint">Заявка уйдёт в корпоративный сервис только после этой кнопки.</p>
    </div>
  )
}

function FieldInput({
  field: f,
  value,
  invalid,
  disabled,
  onChange,
}: {
  field: Field
  value: string
  invalid: boolean
  disabled: boolean
  onChange: (v: string) => void
}) {
  const id = `request-field-${f.key}`
  const common = {
    id,
    name: f.key,
    className: 'request__input',
    required: f.required,
    disabled,
    'aria-invalid': invalid || undefined,
  }
  return (
    <div className="request__field">
      <label htmlFor={id} className="request__label">
        {f.label}
        {f.required && <span aria-hidden="true"> *</span>}
      </label>
      {f.kind === 'choice' ? (
        <select {...common} value={value} onChange={(e) => onChange(e.target.value)}>
          {!f.required && <option value="">—</option>}
          {f.options.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      ) : f.kind === 'long_text' ? (
        <textarea {...common} rows={2} value={value} onChange={(e) => onChange(e.target.value)} />
      ) : (
        <input {...common} value={value} onChange={(e) => onChange(e.target.value)} />
      )}
    </div>
  )
}

const statusView: Record<RequestStatus, { text: string; tone: string }> = {
  REQUEST_STATUS_UNSPECIFIED: { text: 'Отправлено', tone: 'pending' },
  REQUEST_STATUS_PENDING: { text: 'Ждёт согласования', tone: 'pending' },
  REQUEST_STATUS_APPROVED: { text: 'Одобрено', tone: 'approved' },
  REQUEST_STATUS_REJECTED: { text: 'Отклонено', tone: 'rejected' },
}

function RequestStatusCard({ request: r }: { request: ServiceRequest }) {
  const view = statusView[r.status]
  return (
    <section className={`block request request--${view.tone}`} aria-label={`Заявка ${r.id}: ${r.title}`}>
      <p className="block__title">
        {r.title} <span className="request__id">{r.id}</span>
      </p>
      <p className="request__status" aria-live="polite">
        {view.text}
        {r.approver && <span className="block__meta"> · {r.approver.name || roleName(r.approver.role)}</span>}
      </p>
      {r.reason && <p className="block__text">Причина: {r.reason}</p>}
      {r.next_step && <p className="block__meta">{r.next_step}</p>}
    </section>
  )
}
