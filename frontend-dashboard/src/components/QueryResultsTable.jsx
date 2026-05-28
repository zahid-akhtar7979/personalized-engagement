function formatCell(value) {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

export default function QueryResultsTable({ rows, title = 'Query results' }) {
  const data = Array.isArray(rows) ? rows : []
  if (data.length === 0) {
    return (
      <p className="text-netflix-muted text-sm py-2">
        No rows returned. Start <strong>Replay</strong> to populate live event data, or try &quot;Show product catalog&quot;.
      </p>
    )
  }

  const columns = Object.keys(data[0] || {})

  return (
    <div>
      <h3 className="text-sm font-semibold text-netflix-muted mb-2">
        {title} ({data.length} rows)
      </h3>
      <div className="overflow-x-auto rounded-lg border border-white/5">
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-netflix-card">
              {columns.map((col) => (
                <th key={col} className="px-4 py-2 text-left text-netflix-muted font-medium whitespace-nowrap">
                  {col}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.map((row, i) => (
              <tr key={i} className="border-t border-white/5 hover:bg-white/5">
                {columns.map((col) => (
                  <td key={col} className="px-4 py-2 whitespace-nowrap">
                    {formatCell(row[col])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
