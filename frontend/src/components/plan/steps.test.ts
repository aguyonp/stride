import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ActivityTrack, WorkoutStep } from '../../api/types'
import { projectTrack } from '../session/GpsTrack'
import { Markdown } from '../ui/Markdown'
import { parseDuration } from './StepsEditor'
import { stepsSummary, targetText, totalDurationS } from './steps'

const steps: WorkoutStep[] = [
  { type: 'warmup', durationS: 600, target: { kind: 'pace', min: 330, max: 360 } },
  { type: 'repeat', repeat: 6, steps: [{ type: 'interval', durationS: 60, target: { kind: 'pace', min: 250, max: 260 } }, { type: 'recovery', durationS: 90 }] },
  { type: 'cooldown', durationS: 600 },
]

describe('workout steps', () => {
  it('summarises and totals a structured workout', () => {
    expect(stepsSummary(steps)).toBe('Éch. 10′ @5:30–6:00/km · 6 × (1′ @4:10–4:20/km + 1′30 récup) · Retour 10′')
    expect(totalDurationS(steps)).toBe(600 + 6 * 150 + 600)
    expect(totalDurationS([{ type: 'run', distanceM: 1000 }])).toBeNull()
    expect(targetText({ kind: 'hr', min: 130, max: 145 })).toBe('130–145 bpm')
  })

  it('parses durations', () => {
    expect(parseDuration('1:30')).toBe(90)
    expect(parseDuration('10')).toBe(600)
    expect(parseDuration("1'30")).toBe(90)
    expect(parseDuration('abc')).toBeNull()
  })
})

describe('markdown', () => {
  it('renders blocks safely', () => {
    const html = renderToStaticMarkup(createElement(Markdown, { text: '## Titre\n\n- **gras** et `code`\n- [lien](https://exemple.fr) [js](javascript:alert(1))\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n<script>x</script>' }))
    expect(html).toContain('<h3>Titre</h3>')
    expect(html).toContain('<strong>gras</strong>')
    expect(html).toContain('<td>1</td>')
    expect(html).toContain('href="https://exemple.fr"')
    expect(html).not.toContain('javascript:')
    expect(html).not.toContain('<script>')
  })
})

describe('gps projection', () => {
  it('splits the trace into highlighted runs', () => {
    const t: ActivityTrack = {
      points: [
        [45, 3, 0, 175, 0],
        [45.001, 3.001, 10, 160, -3],
        [45.002, 3.0, 20, 160, -3],
        [45.0, 3.0, 30, 176, 1],
      ],
      bounds: { minLat: 45, maxLat: 45.002, minLon: 3, maxLon: 3.001 },
    }
    const c = projectTrack(t, 'cadence', 168)
    expect(c.runs.map((r) => r.hot)).toEqual([false, true, false])
    expect(c.hotPct).toBe(50)
    expect(projectTrack(t, 'slope', 168).runs.filter((r) => r.hot)).toHaveLength(1)
  })
})
