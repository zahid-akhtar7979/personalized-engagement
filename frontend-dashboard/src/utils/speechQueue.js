/**
 * Sequential TTS queue — plays prompts one after another without cutting off mid-sentence.
 */

let queue = []
let pumping = false
let generation = 0

function pickVoice(utterance) {
  const voices = window.speechSynthesis.getVoices()
  const preferred =
    voices.find((v) => v.lang.startsWith('en') && v.name.includes('Google')) ||
    voices.find((v) => v.lang.startsWith('en'))
  if (preferred) utterance.voice = preferred
}

function pump() {
  if (pumping || queue.length === 0 || typeof window === 'undefined' || !window.speechSynthesis) {
    return
  }

  const item = queue[0]
  if (item.gen !== generation) {
    queue.shift()
    item.resolve()
    pump()
    return
  }

  pumping = true
  const utterance = new SpeechSynthesisUtterance(item.text)
  utterance.rate = item.rate ?? 1
  utterance.pitch = item.pitch ?? 1
  utterance.lang = 'en-US'
  pickVoice(utterance)

  const finish = () => {
    pumping = false
    if (queue[0] === item) {
      queue.shift()
    }
    item.resolve()
    pump()
  }

  utterance.onend = finish
  utterance.onerror = finish

  // Do not cancel — let the current phrase finish; queue handles order.
  window.speechSynthesis.speak(utterance)
}

/** Stop immediately and discard pending phrases (new mic session / new query). */
export function clearSpeechQueue() {
  generation += 1
  queue = []
  pumping = false
  window.speechSynthesis?.cancel()
}

/** Queue a phrase; resolves when it has been spoken (or skipped). */
export function enqueueSpeech(text, { rate = 1, pitch = 1 } = {}) {
  const trimmed = text?.trim()
  if (!trimmed) return Promise.resolve()

  const gen = generation
  return new Promise((resolve) => {
    queue.push({ text: trimmed, rate, pitch, resolve, gen })
    pump()
  })
}

/** Wait until the queue is empty and nothing is playing. */
export function drainSpeechQueue() {
  return new Promise((resolve) => {
    const check = () => {
      if (!pumping && queue.length === 0) {
        resolve()
        return
      }
      setTimeout(check, 80)
    }
    check()
  })
}
