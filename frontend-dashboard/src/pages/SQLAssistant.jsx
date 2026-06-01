import { useState } from 'react'
import { runAIQuery } from '../api/client'
import QueryResultsTable from '../components/QueryResultsTable'

const EXAMPLES = [
  'Show top retained users',
  'Show highest conversion categories',
  'Show active users this week',
  'Show recent purchases',
]

export default function SQLAssistant() {
  const [question, setQuestion] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)

  const run = async (q) => {
    const query = q || question
    if (!query.trim()) return
    setLoading(true)
    try {
      const { data } = await runAIQuery(query)
      setResult({
        generatedSql: data.generatedSql,
        rows: data.data ?? [],
        rowCount: data.rowCount ?? (data.data?.length ?? 0),
        summary: data.summary,
      })
    } catch (err) {
      setResult({
        generatedSql: '',
        rows: [{ error: err.response?.data?.error || err.message }],
        rowCount: 0,
      })
    } finally {
      setLoading(false)
    }
  }

  const resultRows = result?.rows ?? result?.data ?? []

  return (
    <div>
      <h1 className="text-2xl font-bold mb-2">AI Assistant</h1>
      <p className="text-netflix-muted text-sm mb-6">
        Ask business questions in natural language — get SQL and results instantly
      </p>

      <div className="bg-netflix-card rounded-xl p-4 border border-white/5 mb-4">
        <textarea
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="e.g. Show top retained users"
          className="w-full bg-black/40 border border-white/10 rounded-lg p-3 text-sm min-h-[80px] resize-y focus:outline-none focus:ring-1 focus:ring-netflix-accent"
        />
        <div className="flex flex-wrap gap-2 mt-3">
          <button
            onClick={() => run()}
            disabled={loading}
            className="px-4 py-2 bg-netflix-accent rounded font-medium text-sm hover:bg-red-600 disabled:opacity-50"
          >
            {loading ? 'Running...' : 'Run Query'}
          </button>
          {EXAMPLES.map((ex) => (
            <button
              key={ex}
              onClick={() => { setQuestion(ex); run(ex) }}
              className="px-3 py-1.5 text-xs rounded bg-white/5 hover:bg-white/10 text-netflix-muted"
            >
              {ex}
            </button>
          ))}
        </div>
      </div>

      {result?.generatedSql && (
        <div className="mb-4">
          <h3 className="text-sm font-semibold text-netflix-muted mb-2">Generated SQL</h3>
          <pre className="bg-black/50 rounded-lg p-4 text-sm overflow-x-auto text-green-400 border border-white/5">
            {result.generatedSql}
          </pre>
        </div>
      )}

      {result && !loading && (
        <QueryResultsTable rows={resultRows} title="Results" />
      )}
    </div>
  )
}
