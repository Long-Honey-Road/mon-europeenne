import { HttpResponse, http } from 'msw'
import type { RequestType, ServiceRequest } from '../api/schemas'

// A fake request service: the catalog, the caller's requests and submission with the backend's error shape.
const API = 'http://api.test'

export const requestTypes: RequestType[] = [
  {
    key: 'access.vpn',
    title: 'Доступ к корпоративному VPN',
    category: 'access',
    fields_schema: {
      type: 'object',
      additionalProperties: false,
      required: ['justification'],
      properties: { justification: { type: 'string', title: 'Обоснование', minLength: 10 } },
    },
    approval_chain: [{ resolver: 'devops', object: '' }],
    sla_hours: 24,
  },
  {
    key: 'access.jira_project',
    title: 'Доступ к проекту Jira',
    category: 'access',
    fields_schema: {
      type: 'object',
      additionalProperties: false,
      required: ['project_key', 'level', 'justification'],
      properties: {
        project_key: { type: 'string', title: 'Ключ проекта' },
        level: { enum: ['read', 'write'], title: 'Уровень' },
        justification: { type: 'string', title: 'Обоснование', minLength: 10 },
        comment: { type: 'string' },
      },
    },
    approval_chain: [{ resolver: 'object_owner', object: 'jira_project' }],
    sla_hours: 24,
  },
]

type State = {
  types: RequestType[]
  requests: ServiceRequest[]
  submitted: { thread_id: string; type_key: string; fields: Record<string, unknown> }[]
  failTypes: boolean
  /** Message of a 400 the next submission gets. */
  rejectSubmit: string | null
  seq: number
}

const fresh = (): State => ({ types: requestTypes, requests: [], submitted: [], failTypes: false, rejectSubmit: null, seq: 0 })

export const requests: State & { reset: () => void } = Object.assign(fresh(), {
  reset() {
    Object.assign(requests, fresh())
  },
})

export function pendingRequest(threadId: string, typeKey: string, fields: Record<string, unknown>): ServiceRequest {
  const type = requests.types.find((t) => t.key === typeKey)
  return {
    id: `REQ-${++requests.seq}`,
    type_key: typeKey,
    title: type?.title ?? typeKey,
    status: 'REQUEST_STATUS_PENDING',
    approver: { role: 'devops', person_id: 'p-igor', name: 'Игорь Волков' },
    next_step: 'Ждём решения DevOps, обычно до суток',
    reason: '',
    fields,
    history: [],
    thread_id: threadId,
    updated_at: '2026-11-13T10:00:00Z',
  }
}

export const requestHandlers = [
  http.get(`${API}/v1/private/request-types`, () =>
    requests.failTypes
      ? HttpResponse.json({ code: 14, message: 'down', details: [] }, { status: 503 })
      : HttpResponse.json({ types: requests.types }),
  ),
  http.get(`${API}/v1/private/requests`, () => HttpResponse.json({ requests: requests.requests })),
  http.post(`${API}/v1/private/requests`, async ({ request }) => {
    const body = (await request.json()) as State['submitted'][number]
    requests.submitted.push(body)
    if (requests.rejectSubmit) {
      return HttpResponse.json({ code: 3, message: requests.rejectSubmit, details: [] }, { status: 400 })
    }
    const r = pendingRequest(body.thread_id, body.type_key, body.fields)
    requests.requests = [r, ...requests.requests]
    return HttpResponse.json({ request: r })
  }),
]
