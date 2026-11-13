import { z } from 'zod'

// Runtime mirrors of components in buddy-api.yaml. protojson emits unset messages as null.
const message = <T extends z.ZodType>(schema: T) => schema.nullish().transform((v) => v ?? undefined)

export const roleSchema = z.enum([
  'ROLE_UNSPECIFIED',
  'ROLE_INTERN',
  'ROLE_MENTOR',
  'ROLE_MANAGER',
  'ROLE_HR_BP',
  'ROLE_HR_ADMIN',
])

export const directionSchema = z.enum([
  'DIRECTION_UNSPECIFIED',
  'DIRECTION_BACKEND',
  'DIRECTION_FRONTEND',
  'DIRECTION_QA',
])

export const userSchema = z.object({
  id: z.string(),
  login: z.string(),
  display_name: z.string(),
  role: roleSchema,
  direction: directionSchema,
  team_id: z.string(),
  repos: z.array(z.string()),
  jira_projects: z.array(z.string()),
  location: z.string(),
  start_date: z.string(),
})

export const teamSchema = z.object({
  id: z.string(),
  name: z.string(),
  unit: z.string(),
  cluster: z.string(),
  mattermost_channel: z.string(),
})

export const personRefSchema = z.object({
  id: z.string(),
  name: z.string(),
  role_title: z.string(),
})

export const relationsSchema = z.object({
  mentor: message(personRefSchema),
  manager: message(personRefSchema),
  hr_bp: message(personRefSchema),
})

export const loginResponseSchema = z.object({
  token: z.string().min(1),
  user: userSchema,
})

export const getMeResponseSchema = z.object({
  user: userSchema,
  team: message(teamSchema),
  relations: message(relationsSchema),
})

export const statusSchema = z.object({
  code: z.number(),
  message: z.string(),
})

export const threadTypeSchema = z.enum([
  'THREAD_TYPE_UNSPECIFIED',
  'THREAD_TYPE_GENERAL',
  'THREAD_TYPE_PLAN',
  'THREAD_TYPE_REQUEST',
  'THREAD_TYPE_TASK',
])

export const threadSchema = z.object({
  id: z.string(),
  type: threadTypeSchema,
  title: z.string(),
  created_at: z.string(),
})

const personHint = {
  person_id: z.string().optional(),
  name: z.string().optional(),
  role_title: z.string().optional(),
  reason: z.string().optional(),
  contact: z.string().optional(),
  other_team: z.boolean().optional(),
}

export const escalationStepSchema = z.object({
  level: z.number(),
  role: z.string(),
  person_id: z.string().optional(),
  name: z.string().optional(),
})

export const sourceSchema = z.object({
  ref: z.number(),
  source_id: z.string(),
  title: z.string().optional(),
  url: z.string().optional(),
  kind: z.string().optional(),
})

// Blocks are built by the server; unknown types and fields pass through so new blocks never break the chat.
export const blockSchema = z.looseObject({
  type: z.string(),
  items: z.array(sourceSchema).optional(),
  ...personHint,
  topic: z.string().optional(),
  escalation: z.array(escalationStepSchema).optional(),
  action: z.record(z.string(), z.unknown()).optional(),
})

export const messageRoleSchema = z.enum(['MESSAGE_ROLE_UNSPECIFIED', 'MESSAGE_ROLE_USER', 'MESSAGE_ROLE_ASSISTANT'])

export const chatMessageSchema = z.object({
  id: z.string(),
  thread_id: z.string(),
  role: messageRoleSchema,
  text: z.string(),
  blocks: z.array(blockSchema),
  created_at: z.string(),
})

export const listThreadsResponseSchema = z.object({ threads: z.array(threadSchema) })
export const threadResponseSchema = z.object({ thread: threadSchema })
export const listMessagesResponseSchema = z.object({ messages: z.array(chatMessageSchema) })
export const sendMessageResponseSchema = z.object({ message_id: z.string(), job_id: z.string() })
export const emptySchema = z.object({})

export type Role = z.infer<typeof roleSchema>
export type User = z.infer<typeof userSchema>
export type LoginResponse = z.infer<typeof loginResponseSchema>
export type Me = z.infer<typeof getMeResponseSchema>
export type ThreadType = z.infer<typeof threadTypeSchema>
export type Thread = z.infer<typeof threadSchema>
export type Block = z.infer<typeof blockSchema>
export type Source = z.infer<typeof sourceSchema>
export type EscalationStep = z.infer<typeof escalationStepSchema>
export type ChatMessage = z.infer<typeof chatMessageSchema>
