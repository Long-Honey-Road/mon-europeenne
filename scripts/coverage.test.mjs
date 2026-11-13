import assert from 'node:assert/strict'
import { test } from 'node:test'
import { goToLines, lcovToLines, summarize, toLcov } from './coverage.mjs'

const MODULE = 'github.com/acme/app/backend'

test('Go blocks become lines; a line is covered if any block on it ran', () => {
  const profile = [
    'mode: atomic',
    `${MODULE}/internal/a/a.go:3.10,5.2 2 1`,
    `${MODULE}/internal/a/a.go:5.2,7.3 1 0`,
    // The same block again from another test binary (-coverpkg): counts add up.
    `${MODULE}/internal/a/a.go:9.1,9.20 1 0`,
    `${MODULE}/internal/a/a.go:9.1,9.20 1 4`,
    `${MODULE}/gen/x.pb.go:1.1,2.1 1 0`,
  ].join('\n')
  const files = goToLines(profile, MODULE, 'backend')
  assert.deepEqual([...files.keys()], ['backend/internal/a/a.go', 'backend/gen/x.pb.go'])
  assert.deepEqual(Object.fromEntries(files.get('backend/internal/a/a.go')), {
    3: 1, 4: 1, 5: 1, 6: 0, 7: 0, 9: 4,
  })
})

test('lcov lines keep their counts and paths become repo-relative', () => {
  const lcov = [
    'TN:',
    'SF:/repo/frontend/src/a.ts',
    'DA:1,3',
    'DA:2,0',
    'end_of_record',
    'SF:src/b.ts',
    'DA:4,1',
    'end_of_record',
  ].join('\n')
  const files = lcovToLines(lcov, '/repo/frontend', 'frontend')
  assert.deepEqual(Object.fromEntries(files.get('frontend/src/a.ts')), { 1: 3, 2: 0 })
  assert.deepEqual(Object.fromEntries(files.get('frontend/src/b.ts')), { 4: 1 })
})

test('summary adds parts into one line figure and checks the threshold', () => {
  const go = new Map([['backend/a.go', new Map([[1, 1], [2, 0], [3, 1], [4, 1]])]])
  const web = new Map([['frontend/a.ts', new Map([[1, 1], [2, 1], [3, 1], [4, 1], [5, 1], [6, 0]])]])
  const s = summarize({ Go: go, Frontend: web }, 80)
  assert.deepEqual(s.parts, [
    { name: 'Go', covered: 3, total: 4, percent: 75 },
    { name: 'Frontend', covered: 5, total: 6, percent: 83.33 },
  ])
  assert.deepEqual({ covered: s.covered, total: s.total, percent: s.percent, ok: s.ok }, {
    covered: 8, total: 10, percent: 80, ok: true,
  })
  assert.equal(summarize({ Go: go }, 80).ok, false)
})

test('an empty part does not divide by zero', () => {
  const s = summarize({ Go: new Map() }, 80)
  assert.deepEqual(s.parts[0], { name: 'Go', covered: 0, total: 0, percent: 0 })
  assert.equal(s.ok, false)
})

test('merged lcov lists every file with its line hits', () => {
  const out = toLcov(new Map([['backend/a.go', new Map([[2, 0], [1, 5]])]]))
  assert.equal(out, 'TN:\nSF:backend/a.go\nDA:1,5\nDA:2,0\nLF:2\nLH:1\nend_of_record\n')
})
