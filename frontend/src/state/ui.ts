import { useSyncExternalStore } from 'react'

interface UiState {
  selectedActivityId: number | null
  feelingOpen: boolean
}

let state: UiState = { selectedActivityId: null, feelingOpen: false }
const listeners = new Set<() => void>()

function set(patch: Partial<UiState>) {
  state = { ...state, ...patch }
  listeners.forEach((l) => l())
}

const subscribe = (cb: () => void) => {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

export function useUi<T>(select: (s: UiState) => T): T {
  return useSyncExternalStore(subscribe, () => select(state))
}

export const ui = {
  selectActivity: (id: number | null) => set({ selectedActivityId: id }),
  openFeeling: () => set({ feelingOpen: true }),
  closeFeeling: () => set({ feelingOpen: false }),
}
