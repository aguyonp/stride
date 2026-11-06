// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { resetEventStore, upsertJobInStore, useActiveJobs, useLlmStream } from './events'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let renders = 0
function Probe() {
  renders++
  const jobs = useActiveJobs()
  const stream = useLlmStream(null, 'activity', '1')
  return (
    <span>
      {jobs.length}:{stream ? 'stream' : 'none'}
    </span>
  )
}

afterEach(() => resetEventStore())

describe('event store hooks', () => {
  it('render without an update loop and react to store changes', () => {
    const el = document.createElement('div')
    const root = createRoot(el)
    act(() => root.render(<Probe />))
    expect(el.textContent).toBe('0:none')
    expect(renders).toBeLessThan(5)

    act(() =>
      upsertJobInStore({ id: 'j1', kind: 'sync', queue: 'default', state: 'running', updatedAt: '2026-11-07T00:00:00Z' } as unknown as Parameters<typeof upsertJobInStore>[0]),
    )
    expect(el.textContent).toBe('1:none')
    act(() => root.unmount())
  })
})
