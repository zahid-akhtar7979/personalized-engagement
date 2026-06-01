import { useCallback, useEffect, useRef, useState } from 'react'
import { runAIQuery, getAIStatus } from '../api/client'
import QueryResultsTable from '../components/QueryResultsTable'
import {
  speak,
  stopSpeaking,
  useSpeechRecognition,
} from '../hooks/useSpeechRecognition'
import { ackPhrase, listeningHint, slowPhrase } from '../utils/voicePrompts'

const VOICE_EXAMPLES = [
  'Show top 20 users by number of events',
  'Which categories have the highest purchase conversion rate?',
  'Show the 10 most recent purchases with product titles',
  'How many distinct users are in the database?',
]

const SLOW_QUERY_MS = 4500

export default function VoiceSQLAssistant() {
  const [loading, setLoading] = useState(false)
  const [statusText, setStatusText] = useState('')
  const [result, setResult] = useState(null)
  const [speaking, setSpeaking] = useState(false)
  const [manualQuestion, setManualQuestion] = useState('')
  const [llmStatus, setLlmStatus] = useState(null)
  const queryRunId = useRef(0)

  useEffect(() => {
    getAIStatus()
      .then(({ data }) => setLlmStatus(data))
      .catch(() => setLlmStatus({ llmEnabled: false }))
  }, [])

  const speakSummary = useCallback((text) => {
    if (!text?.trim()) return
    stopSpeaking()
    setSpeaking(true)
    const utterance = speak(text.trim())
    if (utterance) {
      utterance.onend = () => setSpeaking(false)
      utterance.onerror = () => setSpeaking(false)
    } else {
      setSpeaking(false)
    }
  }, [])

  const runQuery = useCallback(
    async (question) => {
      const q = question?.trim()
      if (!q) return

      const runId = ++queryRunId.current
      setLoading(true)
      setResult(null)
      setStatusText('Processing your question…')
      stopSpeaking()

      // Immediate voice acknowledgment (parallel with API)
      speak(ackPhrase(), { cancelPrevious: true })

      const slowTimer = setTimeout(() => {
        if (queryRunId.current !== runId) return
        setStatusText('Still fetching results…')
        speak(slowPhrase(), { cancelPrevious: true })
      }, SLOW_QUERY_MS)

      try {
        setStatusText('Generating SQL with OpenAI and querying PostgreSQL…')
        const { data } = await runAIQuery(q)

        if (queryRunId.current !== runId) return

        clearTimeout(slowTimer)
        stopSpeaking()

        setResult(data)
        setStatusText(
          data.source === 'openai'
            ? 'Done — SQL from OpenAI, results from your database.'
            : 'Done — results ready.'
        )

        const spoken =
          data.summary?.trim() ||
          (data.rowCount > 0
            ? `I found ${data.rowCount} rows for your question.`
            : 'I did not find any rows for that question.')

        speakSummary(spoken)
      } catch (err) {
        if (queryRunId.current !== runId) return

        clearTimeout(slowTimer)
        stopSpeaking()

        const msg = err.response?.data?.error || err.message
        setResult({
          generatedSql: '',
          summary: `Sorry, something went wrong. ${msg}`,
          data: [],
          source: 'error',
        })
        setStatusText('Request failed.')
        speakSummary('Sorry, I could not complete that request. Please try again or rephrase your question.')
      } finally {
        if (queryRunId.current === runId) {
          setLoading(false)
        }
      }
    },
    [speakSummary]
  )

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
    return () => {
      queryRunId.current += 1
      stopSpeaking()
    }
  }, [])

  const displayQuestion = manualQuestion || transcript || interim
  const resultRows = result?.data ?? result?.rows ?? []

  const toggleMic = () => {
    if (listening) {
      stop()
    } else {
      reset()
      setResult(null)
      setStatusText('')
      start()
      speak(listeningHint(), { cancelPrevious: true })
    }
  }

  return (
    <div className="max-w-4xl mx-auto">
      <h1 className="text-2xl font-bold mb-2">AI Voice Assistant</h1>
      <p className="text-netflix-muted text-sm mb-2">
        Speak a business question — we transcribe it, OpenAI writes safe SQL, PostgreSQL returns data, and the assistant reads the answer aloud.
      </p>
      {llmStatus && (
        <p className="text-xs mb-4">
          {llmStatus.llmEnabled ? (
            <span className="text-green-400">
              ● Voice + LLM active ({llmStatus.model || 'gpt-4o-mini'})
            </span>
          ) : (
            <span className="text-yellow-400">
              ○ Mock SQL mode — set OPENAI_API_KEY in .env for OpenAI-generated queries
            </span>
          )}
        </p>
      )}

      {!supported && (
        <div className="mb-4 p-4 rounded-lg bg-yellow-500/10 border border-yellow-500/30 text-yellow-200 text-sm">
          Web Speech API is not supported in this browser. Use Chrome or Edge, or type your question below.
        </div>
      )}

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

        {(loading || statusText) && (
          <div className="mt-3 flex items-center justify-center gap-2 text-sm text-netflix-accent">
            {loading && (
              <span className="inline-block w-2 h-2 rounded-full bg-netflix-accent animate-pulse" />
            )}
            <p className={loading ? 'animate-pulse' : ''}>
              {statusText || 'Working…'}
            </p>
          </div>
        )}
      </div>

      <div className="bg-netflix-card rounded-xl p-4 border border-white/5 mb-6">
        <p className="text-xs text-netflix-muted mb-2">Or type your question</p>
        <div className="flex gap-2 flex-wrap">
          <input
            type="text"
            value={manualQuestion}
            onChange={(e) => setManualQuestion(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && runQuery(manualQuestion)}
            placeholder="How many users viewed products last week?"
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
              disabled={loading}
              className="px-3 py-1 text-xs rounded-full bg-white/5 hover:bg-white/10 text-netflix-muted disabled:opacity-50 text-left"
            >
              {ex}
            </button>
          ))}
        </div>
      </div>

      {result?.summary && (
        <div className="mb-6 p-4 rounded-xl bg-netflix-accent/10 border border-netflix-accent/30 flex items-start gap-3">
          <span className="text-2xl">{speaking ? '🔊' : '💬'}</span>
          <div>
            <p className="text-xs text-netflix-muted uppercase tracking-wide mb-1">Voice response</p>
            <p className="text-lg font-medium">{result.summary}</p>
            {result.summary && !speaking && (
              <button
                type="button"
                onClick={() => speakSummary(result.summary)}
                className="mt-2 text-xs text-netflix-accent hover:underline"
              >
                Replay audio
              </button>
            )}
          </div>
        </div>
      )}

      {result?.generatedSql && (
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-2">
            <h3 className="text-sm font-semibold text-netflix-muted">Generated SQL</h3>
            {result.source === 'openai' && (
              <span className="text-[10px] px-2 py-0.5 rounded bg-green-500/20 text-green-400">OpenAI</span>
            )}
          </div>
          <pre className="bg-black/50 rounded-lg p-4 text-sm overflow-x-auto text-green-400 border border-white/5">
            {result.generatedSql}
          </pre>
        </div>
      )}

      {result && !loading && (
        <QueryResultsTable rows={resultRows} title={`Results (${result.rowCount ?? resultRows.length} rows)`} />
      )}
    </div>
  )
}
