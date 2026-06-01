import { useCallback, useEffect, useRef, useState } from 'react'

function getSpeechRecognition() {
  if (typeof window === 'undefined') return null
  return window.SpeechRecognition || window.webkitSpeechRecognition || null
}

export function useSpeechRecognition({ onResult, onEnd, lang = 'en-US' } = {}) {
  const [listening, setListening] = useState(false)
  const [transcript, setTranscript] = useState('')
  const [interim, setInterim] = useState('')
  const [supported, setSupported] = useState(false)
  const [error, setError] = useState(null)
  const recognitionRef = useRef(null)
  const onResultRef = useRef(onResult)
  const onEndRef = useRef(onEnd)

  onResultRef.current = onResult
  onEndRef.current = onEnd

  useEffect(() => {
    const SR = getSpeechRecognition()
    setSupported(!!SR)
    if (!SR) return

    const recognition = new SR()
    recognition.continuous = false
    recognition.interimResults = true
    recognition.lang = lang
    recognition.maxAlternatives = 1

    recognition.onstart = () => {
      setListening(true)
      setError(null)
    }

    recognition.onresult = (event) => {
      let finalText = ''
      let interimText = ''
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const t = event.results[i][0].transcript
        if (event.results[i].isFinal) {
          finalText += t
        } else {
          interimText += t
        }
      }
      if (interimText) setInterim(interimText.trim())
      if (finalText) {
        const trimmed = finalText.trim()
        setTranscript(trimmed)
        setInterim('')
        onResultRef.current?.(trimmed)
      }
    }

    recognition.onerror = (event) => {
      setError(event.error || 'speech recognition failed')
      setListening(false)
    }

    recognition.onend = () => {
      setListening(false)
      onEndRef.current?.()
    }

    recognitionRef.current = recognition
    return () => {
      try {
        recognition.abort()
      } catch (_) {}
    }
  }, [lang])

  const start = useCallback(() => {
    const recognition = recognitionRef.current
    if (!recognition) {
      setError('Speech recognition not supported in this browser')
      return
    }
    setTranscript('')
    setInterim('')
    setError(null)
    try {
      recognition.start()
    } catch (e) {
      if (e.name === 'InvalidStateError') {
        recognition.stop()
        setTimeout(() => recognition.start(), 100)
      } else {
        setError(e.message)
      }
    }
  }, [])

  const stop = useCallback(() => {
    recognitionRef.current?.stop()
  }, [])

  const reset = useCallback(() => {
    setTranscript('')
    setInterim('')
    setError(null)
  }, [])

  return {
    supported,
    listening,
    transcript,
    interim,
    error,
    start,
    stop,
    reset,
  }
}

export function speak(text, { rate = 1, pitch = 1, cancelPrevious = true } = {}) {
  if (typeof window === 'undefined' || !window.speechSynthesis || !text) return null

  if (cancelPrevious) {
    window.speechSynthesis.cancel()
  }
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.rate = rate
  utterance.pitch = pitch
  utterance.lang = 'en-US'

  const voices = window.speechSynthesis.getVoices()
  const preferred = voices.find((v) => v.lang.startsWith('en') && v.name.includes('Google'))
    || voices.find((v) => v.lang.startsWith('en'))
  if (preferred) utterance.voice = preferred

  window.speechSynthesis.speak(utterance)
  return utterance
}

/** Resolves when speech finishes or fails (for chaining voice prompts). */
export function speakAsync(text, options = {}) {
  return new Promise((resolve) => {
    const utterance = speak(text, options)
    if (!utterance) {
      resolve()
      return
    }
    const done = () => resolve()
    utterance.onend = done
    utterance.onerror = done
  })
}

export function stopSpeaking() {
  window.speechSynthesis?.cancel()
}

export function pickRandom(list) {
  return list[Math.floor(Math.random() * list.length)]
}
