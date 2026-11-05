import { useCallback, useState } from 'react'
import { readStored, writeStored } from '../lib/storage'

/** localStorage-backed state, validated on read. */
export function useStoredState<T>(
  key: string,
  fallback: T,
  parse: (raw: string) => T | undefined,
  serialize: (v: T) => string = String,
): [T, (v: T) => void] {
  const [value, setValue] = useState<T>(() => {
    const raw = readStored(key)
    if (raw === null) return fallback
    const parsed = parse(raw)
    return parsed === undefined ? fallback : parsed
  })
  const set = useCallback(
    (v: T) => {
      setValue(v)
      writeStored(key, serialize(v))
    },
    [key, serialize],
  )
  return [value, set]
}
