/** Russian noun form for a count: forms are for 1, 2 and 5 (шаг, шага, шагов). */
export function plural(n: number, [one, few, many]: readonly [string, string, string]): string {
  const d = Math.abs(n) % 100
  if (d >= 11 && d <= 14) return many
  if (d % 10 === 1) return one
  if (d % 10 >= 2 && d % 10 <= 4) return few
  return many
}

/** Genitive after «из N»: «из 1 шага», «из 21 шага», otherwise «из N шагов». */
export function pluralGenitive(n: number, [one, many]: readonly [string, string]): string {
  const d = Math.abs(n) % 100
  return d % 10 === 1 && d !== 11 ? one : many
}
