import { useCallback, useEffect, useState } from 'react'
import { runAIQuery } from '../api/client'
import QueryResultsTable from '../components/QueryResultsTable'
import { speak, stopSpeaking, useSpeechRecognition } from '../hooks/useSpeechRecognition'

const VOICE_EXAMPLES = [
  'Show top retained users',
  'Show highest conversion categories',
  'Show active users this week',
  'Show recent purchases',
]

export default function VoiceSQLAssistant() {
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)
  const [speaking, setSpeaking] = useState(false)
  const [manualQuestion, setManualQuestion] = useState('')

  const runQuery = useCallback(async (question) => {
    const q = question?.trim()
    if (!q) return

    setLoading(true)
    setResult(null)
    stopSpeaking()

    try {
      const { data } = await runAIQuery(q)
      setResult(data)
      if (data.summary) {
        setSpeaking(true)
        const utterance = speak(data.summary)
        if (utterance) {
          utterance.onend = () => setSpeaking(false)
          utterance.onerror = () => setSpeaking(false)
        } else {
          setSpeaking(false)
        }
      }
    } catch (err) {
      const msg = err.response?.data?.error || err.message
      setResult({
        generatedSql: '',
        summary: `Sorry, something went wrong. ${msg}`,
        data: [],
      })
      speak(`Sorry, the query failed.`)
    } finally {
      setLoading(false)
    }
  }, [])

  const handleFinalTranscript = useCallback(
    (text) => {
      setManualQuestion(text)
      runQuery(text)
    },
    [runQuery]
  )

  const {
    supported,
    listening,
    transcript,
    interim,
    error: speechError,
    start,
    stop,
    reset,
  } = useSpeechRecognition({
    onResult: handleFinalTranscript,
  })

  useEffect(() => {
    if (typeof window !== 'undefined' && window.speechSynthesis) {
      window.speechSynthesis.getVoices()
    }
    return () => stopSpeaking()
  }, [])

  const displayQuestion = manualQuestion || transcript || interim
  const resultRows = result?.data ?? result?.rows ?? []

  const toggleMic = () => {
    if (listening) {
      stop()
    } else {
      reset()
      setResult(null)
      start()
    }
  }

  return (
    <div className="max-w-4xl mx-auto">
      <h1 className="text-2xl font-bold mb-2">AI Voice SQL Assistant</h1>
      <p className="text-netflix-muted text-sm mb-6">
        Speak a business question — we transcribe it, generate safe SQL, run it on PostgreSQL, and read the answer aloud.
      </p>

      {!supported && (
        <div className="mb-4 p-4 rounded-lg bg-yellow-500/10 border border-yellow-500/30 text-yellow-200 text-sm">
          Web Speech API is not supported in this browser. Use Chrome or Edge, or type your question below.
        </div>
      )}

      {/* Microphone + live transcription */}
      <div className="bg-netflix-card rounded-xl p-6 border border-white/5 mb-6 text-center">
        <button
          type="button"
          onClick={toggleMic}
          disabled={!supported || loading}
          className={`relative w-24 h-24 rounded-full mx-auto flex items-center justify-center transition-all ${
            listening
              ? 'bg-netflix-accent animate-pulse shadow-lg shadow-netflix-accent/40'
              : 'bg-white/10 hover:bg-netflix-accent/80'
          } disabled:opacity-50`}
          aria-label={listening ? 'Stop listening' : 'Start microphone'}
        >
          <span className="text-4xl">{listening ? '⏹' : '🎤'}</span>
          {listening && (
            <span className="absolute inset-0 rounded-full border-2 border-netflix-accent animate-ping opacity-50" />
          )}
        </button>

        <p className="mt-4 text-sm text-netflix-muted">
          {listening ? 'Listening… speak your question' : 'Tap microphone and ask a question'}
        </p>

        <div className="mt-4 min-h-[3rem] p-4 rounded-lg bg-black/40 border border-white/10 text-left">
          <p className="text-xs text-netflix-muted uppercase tracking-wide mb-1">Live transcription</p>
          <p className="text-base">
            {displayQuestion || (
              <span className="text-netflix-muted italic">Your words will appear here…</span>
            )}
            {interim && listening && (
              <span className="text-netflix-muted"> {interim}</span>
            )}
          </p>
        </div>

        {speechError && (
          <p className="mt-2 text-sm text-red-400">{speechError}</p>
        )}

        {loading && (
          <p className="mt-3 text-sm text-netflix-accent animate-pulse">Generating SQL and running query…</p>
        )}
      </div>

      {/* Manual fallback */}
      <div className="bg-netflix-card rounded-xl p-4 border border-white/5 mb-6">
        <p className="text-xs text-netflix-muted mb-2">Or type your question</p>
        <div className="flex gap-2 flex-wrap">
          <input
            type="text"
            value={manualQuestion}
            onChange={(e) => setManualQuestion(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && runQuery(manualQuestion)}
            placeholder="Show top retained users"
            className="flex-1 min-w-[200px] bg-black/40 border border-white/10 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-netflix-accent"
          />
          <button
            type="button"
            onClick={() => runQuery(manualQuestion)}
            disabled={loading}
            className="px-4 py-2 bg-netflix-accent rounded-lg text-sm font-medium disabled:opacity-50"
          >
            Ask
          </button>
        </div>
        <div className="flex flex-wrap gap-2 mt-3">
          {VOICE_EXAMPLES.map((ex) => (
            <button
              key={ex}
              type="button"
              onClick={() => {
                setManualQuestion(ex)
                runQuery(ex)
              }}
              className="px-3 py-1 text-xs rounded-full bg-white/5 hover:bg-white/10 text-netflix-muted"
            >
              {ex}
            </button>
          ))}
        </div>
      </div>

      {/* Voice response */}
      {result?.summary && (
        <div className="mb-6 p-4 rounded-xl bg-netflix-accent/10 border border-netflix-accent/30 flex items-start gap-3">
          <span className="text-2xl">{speaking ? '🔊' : '💬'}</span>
          <div>
            <p className="text-xs text-netflix-muted uppercase tracking-wide mb-1">Voice response</p>
            <p className="text-lg font-medium">{result.summary}</p>
            {result.summary && !speaking && (
              <button
                type="button"
                onClick={() => {
                  setSpeaking(true)
                  const u = speak(result.summary)
                  if (u) {
                    u.onend = () => setSpeaking(false)
                  } else {
                    setSpeaking(false)
                  }
                }}
                className="mt-2 text-xs text-netflix-accent hover:underline"
              >
                Replay audio
              </button>
            )}
          </div>
        </div>
      )}

      {/* Generated SQL */}
      {result?.generatedSql && (
        <div className="mb-6">
          <h3 className="text-sm font-semibold text-netflix-muted mb-2">Generated SQL</h3>
          <pre className="bg-black/50 rounded-lg p-4 text-sm overflow-x-auto text-green-400 border border-white/5">
            {result.generatedSql}
          </pre>
        </div>
      )}

      {/* Results table — always show after a query completes */}
      {result && !loading && (
        <QueryResultsTable rows={resultRows} />
      )}
    </div>
  )
}
