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

export type Role = z.infer<typeof roleSchema>
export type User = z.infer<typeof userSchema>
export type LoginResponse = z.infer<typeof loginResponseSchema>
export type Me = z.infer<typeof getMeResponseSchema>
