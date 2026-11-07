import { useSyncExternalStore } from 'react'

interface UiState {
  selectedActivityId: number | null
  feelingOpen: boolean
  commandOpen: boolean
  manualActivityOpen: boolean
  dayAnalysisOpen: boolean
  dayAnalysisRun: number
  feedbackOpen: boolean
}

const INITIAL: UiState = {
  selectedActivityId: null,
  feelingOpen: false,
  commandOpen: false,
  manualActivityOpen: false,
  dayAnalysisOpen: false,
  dayAnalysisRun: 0,
  feedbackOpen: false,
}

let state: UiState = INITIAL
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
  openCommand: () => set({ commandOpen: true }),
  closeCommand: () => set({ commandOpen: false }),
  toggleCommand: () => set({ commandOpen: !state.commandOpen }),
  openManualActivity: () => set({ manualActivityOpen: true }),
  closeManualActivity: () => set({ manualActivityOpen: false }),
  triggerDayAnalysis: () => set({ dayAnalysisOpen: true, dayAnalysisRun: state.dayAnalysisRun + 1 }),
  closeDayAnalysis: () => set({ dayAnalysisOpen: false }),
  openFeedback: () => set({ feedbackOpen: true }),
  closeFeedback: () => set({ feedbackOpen: false }),
  reset: () => set(INITIAL),
}
