const roleNames: Record<string, string> = {
  mentor: 'наставник',
  manager: 'руководитель',
  hr_bp: 'HR BP',
  open_sources: 'открытые источники',
  object_owner: 'владелец ресурса',
  devops: 'DevOps',
  hr_admin: 'HR-администратор',
}

/** Human name of an escalation or approval role; unknown roles pass through. */
export const roleName = (role: string) => roleNames[role] ?? role
