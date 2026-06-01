import { pickRandom } from '../hooks/useSpeechRecognition'

/** Spoken immediately after the user finishes their question. */
export const VOICE_ACK_PHRASES = [
  'Got it. One moment while I fetch that from your data.',
  'Sure. I am looking that up in the database now.',
  'Understood. Let me generate the query and pull the results.',
  'Okay. I will check your Retailrocket data and get back to you.',
]

/** Spoken if the OpenAI + database call is still running after a few seconds. */
export const VOICE_SLOW_PHRASES = [
  'Still working on your request. Large datasets can take a few seconds.',
  'I am still fetching the details. Almost there.',
  'Hang on. I am running the query against your event history now.',
]

/** Brief confirmation when the microphone starts. */
export const VOICE_LISTENING_HINTS = [
  'I am listening. Go ahead with your question.',
  'Listening now. What would you like to know?',
]

export function ackPhrase() {
  return pickRandom(VOICE_ACK_PHRASES)
}

export function slowPhrase() {
  return pickRandom(VOICE_SLOW_PHRASES)
}

export function listeningHint() {
  return pickRandom(VOICE_LISTENING_HINTS)
}
