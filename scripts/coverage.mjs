// Merges Go and frontend coverage into one line figure for the whole application.
// Usage: node scripts/coverage.mjs <go profile> <frontend lcov> <out dir> [min %]
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

/** Go cover profile → file → line → hits. A line counts as covered if any block over it ran. */
export function goToLines(profile, modulePath, dir) {
  const blocks = new Map()
  for (const raw of profile.split('\n')) {
    const line = raw.trim()
    if (!line || line.startsWith('mode:')) continue
    const m = /^(.+):(\d+)\.\d+,(\d+)\.\d+ \d+ (\d+)$/.exec(line)
    if (!m) continue
    const key = `${m[1]}:${m[2]}:${m[3]}:${line.split(' ')[0]}`
    blocks.set(key, { file: m[1], from: Number(m[2]), to: Number(m[3]), hits: (blocks.get(key)?.hits ?? 0) + Number(m[4]) })
  }
  const files = new Map()
  for (const b of blocks.values()) {
    const name = b.file.startsWith(`${modulePath}/`) ? `${dir}/${b.file.slice(modulePath.length + 1)}` : b.file
    if (!files.has(name)) files.set(name, new Map())
    const lines = files.get(name)
    for (let n = b.from; n <= b.to; n++) lines.set(n, Math.max(lines.get(n) ?? 0, b.hits))
  }
  return files
}

/** lcov → file → line → hits, with paths relative to the repository. */
export function lcovToLines(lcov, root, dir) {
  const files = new Map()
  let current = null
  for (const raw of lcov.split('\n')) {
    const line = raw.trim()
    if (line.startsWith('SF:')) {
      const path = line.slice(3)
      const rel = path.startsWith('/') ? relative(root, path) : path
      const name = `${dir}/${rel}`
      current = files.get(name) ?? new Map()
      files.set(name, current)
    } else if (line.startsWith('DA:') && current) {
      const [n, hits] = line.slice(3).split(',').map(Number)
      current.set(n, Math.max(current.get(n) ?? 0, hits))
    } else if (line === 'end_of_record') {
      current = null
    }
  }
  return files
}

const round = (x) => Math.round(x * 100) / 100

function count(files) {
  let covered = 0
  let total = 0
  for (const lines of files.values()) {
    for (const hits of lines.values()) {
      total++
      if (hits > 0) covered++
    }
  }
  return { covered, total }
}

/** Per-part and total line coverage, and whether the total reaches `min` percent. */
export function summarize(parts, min) {
  const rows = Object.entries(parts).map(([name, files]) => {
    const { covered, total } = count(files)
    return { name, covered, total, percent: total ? round((covered / total) * 100) : 0 }
  })
  const covered = rows.reduce((s, r) => s + r.covered, 0)
  const total = rows.reduce((s, r) => s + r.total, 0)
  const percent = total ? round((covered / total) * 100) : 0
  return { parts: rows, covered, total, percent, ok: total > 0 && percent >= min }
}

export function toLcov(files) {
  let out = ''
  for (const [name, lines] of files) {
    const sorted = [...lines.entries()].sort((a, b) => a[0] - b[0])
    out += `TN:\nSF:${name}\n`
    for (const [n, hits] of sorted) out += `DA:${n},${hits}\n`
    out += `LF:${sorted.length}\nLH:${sorted.filter(([, h]) => h > 0).length}\nend_of_record\n`
  }
  return out
}

// Kept in step with backend/Makefile (COVER_PKGS) and frontend/vite.config.ts (coverage.exclude).
export const EXCLUDED = [
  ['Go', 'backend/gen/** (generated protobuf, gRPC and gateway code)'],
  ['Go', 'backend/internal/mocks/** (mock services, by the case rules)'],
  ['Go', '*_test.go (tests themselves)'],
  ['Frontend', 'frontend/src/api/schema.gen.ts (generated from OpenAPI)'],
  ['Frontend', 'frontend/src/test/** (test fakes and the dev mock API)'],
  ['Frontend', 'frontend/src/main.tsx (entry point), *.test.ts(x)'],
]

function report(s, min) {
  const rows = s.parts.map((p) => `| ${p.name} | ${p.covered} | ${p.total} | ${p.percent}% |`).join('\n')
  const excluded = EXCLUDED.map(([part, what]) => `- ${part}: ${what}`).join('\n')
  return `# Coverage

Line coverage of the application code, Go and frontend together. Threshold: ${min}%.

| Part | Covered lines | Lines | Coverage |
|---|---|---|---|
${rows}
| **Total** | **${s.covered}** | **${s.total}** | **${s.percent}%** |

Result: ${s.ok ? 'passes' : 'BELOW THRESHOLD'}.

Excluded from the count:

${excluded}

Merged lcov: \`coverage/lcov.info\`. Go profile: \`coverage/go.out\`. Frontend HTML: \`coverage/frontend/index.html\`.
`
}

function main([goProfile, webLcov, outDir, minArg]) {
  const repo = resolve(fileURLToPath(new URL('..', import.meta.url)))
  const modulePath = /^module (.+)$/m.exec(readFileSync(join(repo, 'backend/go.mod'), 'utf8'))[1].trim()
  const go = goToLines(readFileSync(goProfile, 'utf8'), modulePath, 'backend')
  const web = lcovToLines(readFileSync(webLcov, 'utf8'), join(repo, 'frontend'), 'frontend')
  const min = Number(minArg ?? 80)
  const s = summarize({ Go: go, Frontend: web }, min)

  mkdirSync(outDir, { recursive: true })
  writeFileSync(join(outDir, 'lcov.info'), toLcov(new Map([...go, ...web])))
  const md = report(s, min)
  writeFileSync(join(outDir, 'summary.md'), md)
  process.stdout.write(md)
  if (!s.ok) process.exitCode = 1
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main(process.argv.slice(2))
