export const meta = {
  name: 'atlas-llm-qa-v2',
  description: 'Real-LLM code-Q&A (post callers-fix): Atlas vs graphify vs raw-file, scored vs gopls truth',
  phases: [{ title: 'Load' }, { title: 'Answer' }],
}

phase('Load')
const setup = await agent(
  'Run exactly this shell command: cat /tmp/llmqa_wfargs.json\nThen return ONLY the raw file contents (valid JSON), nothing else — no markdown fences, no commentary.',
  { label: 'load-tasks' }
)
let tasks
try { tasks = JSON.parse(setup).tasks } catch (e) {
  const m = setup.match(/\{[\s\S]*\}/)
  tasks = JSON.parse(m[0]).tasks
}
log(`loaded ${tasks.length} tasks (${new Set(tasks.map(t=>t.symbol)).size} symbols x 3 conditions)`)

const ANS = {
  type: 'object',
  properties: { names: { type: 'array', items: { type: 'string' } } },
  required: ['names'],
  additionalProperties: false,
}

phase('Answer')
const answered = await parallel(tasks.map((t) => () =>
  agent(
    `Step 1: run this shell command to read the CONTEXT block (the output of a code-intelligence tool):\n` +
    `cat ${t.ctxFile}\n\n` +
    `Step 2: Using ONLY that context — no outside knowledge, no web, do NOT open or cat any other file — ` +
    `list the names of the functions that DIRECTLY call \`${t.symbol}\`.\n` +
    `If the context does not contain caller information, return an empty list.\n` +
    `Return JSON: {"names": [...]} with just the bare function names.`,
    { label: `${t.cond}:${t.symbol}`, phase: 'Answer', schema: ANS }
  ).then((a) => ({ symbol: t.symbol, cond: t.cond, ctxTok: t.ctxTok, truth: t.truth, answer: (a && a.names) || [] }))
))

// Deterministic scoring (NOT an LLM): recall/precision/F1 of answer-set vs gopls truth.
const norm = (s) => String(s).trim().replace(/\(\)$/, '').toLowerCase()
function score(answer, truth) {
  const A = new Set(answer.map(norm)), T = new Set(truth.map(norm))
  let hit = 0
  for (const a of A) if (T.has(a)) hit++
  const recall = T.size ? hit / T.size : 0
  const precision = A.size ? hit / A.size : 0
  const f1 = (recall + precision) ? (2 * recall * precision) / (recall + precision) : 0
  return { hit, nAnswer: A.size, nTruth: T.size, recall, precision, f1 }
}

const perQ = answered.filter(Boolean).map((r) => ({ ...r, ...score(r.answer, r.truth) }))
const byCond = {}
for (const c of ['atlas', 'graphify', 'baseline']) {
  const rows = perQ.filter((r) => r.cond === c)
  const avg = (k) => rows.reduce((s, r) => s + r[k], 0) / (rows.length || 1)
  byCond[c] = {
    n: rows.length,
    avgRecall: +avg('recall').toFixed(3),
    avgPrecision: +avg('precision').toFixed(3),
    avgF1: +avg('f1').toFixed(3),
    avgCtxTok: Math.round(avg('ctxTok')),
  }
}

return {
  summary: byCond,
  perQuestion: perQ.map((r) => ({
    symbol: r.symbol, cond: r.cond, hit: r.hit, nAnswer: r.nAnswer, nTruth: r.nTruth,
    recall: +r.recall.toFixed(2), precision: +r.precision.toFixed(2), f1: +r.f1.toFixed(2), ctxTok: r.ctxTok,
  })),
}
