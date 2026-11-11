import { describe, expect, it } from 'vitest'
import type { BodyCorrelationFactor } from '../../api/types'
import {
  correlationConfidence,
  correlationSample,
  fmtMeasure,
  isCorrelationInsufficient,
  shoeHighlightDetail,
} from './bodyMapMeta'
import { setLang } from '../../i18n/lang'

// These tests assert the French UI copy.
setLang('fr', { persist: false })

const factor = (over: Partial<BodyCorrelationFactor> = {}): BodyCorrelationFactor => ({
  zone: 'knee_pfps',
  zoneLabel: 'Rotule / Genou (SFP)',
  factor: 'elev_loss',
  factorLabel: 'Dénivelé négatif (D−)',
  unit: 'm',
  symptomaticMean: 168,
  baselineMean: 54,
  deltaPct: 211.1,
  symptomaticRuns: 3,
  baselineRuns: 14,
  confidence: 'high',
  shoeHighlight: null,
  summary: 'Plus de D− lors des épisodes.',
  ...over,
})

describe('body map correlation helpers', () => {
  it('flags a factor as insufficient when the backend says so or no comparison value exists', () => {
    expect(isCorrelationInsufficient(factor())).toBe(false)
    expect(isCorrelationInsufficient(factor({ insufficientData: true }))).toBe(true)
    expect(isCorrelationInsufficient(factor({ symptomaticMean: null, baselineMean: null, deltaPct: null }))).toBe(true)
    expect(isCorrelationInsufficient(factor({ baselineMean: null, deltaPct: null }))).toBe(false)
  })

  it('never exposes a confidence that is null or contradicts insufficient data', () => {
    expect(correlationConfidence(factor())?.label).toBe('Fiabilité élevée')
    expect(correlationConfidence(factor({ confidence: null }))).toBeNull()
    expect(correlationConfidence(factor({ confidence: 'high', insufficientData: true }))).toBeNull()
    expect(correlationConfidence(factor({ confidence: 'insufficient' }))?.label).toBe('Données insuffisantes')
  })

  it('formats measures and samples only from real values', () => {
    expect(fmtMeasure(168, 'm')).toBe('168 m')
    expect(fmtMeasure(null, 'm')).toBeNull()
    expect(correlationSample(factor())).toBe('3 avec gêne · 14 réf.')
    expect(correlationSample(factor({ baselineRuns: null }))).toBeNull()
    expect(correlationSample(factor({ symptomaticRuns: null }))).toBeNull()
  })

  it('omits unknown shoe wear, drop and episode counts instead of inventing them', () => {
    const shoe = { shoeId: 1, shoeName: 'Pegasus 41', dropMm: 10, distanceKm: 584, painEpisodes: 2 }
    expect(shoeHighlightDetail(shoe)).toBe('Drop 10 mm · 584 km · 2 épisodes')
    expect(shoeHighlightDetail({ ...shoe, painEpisodes: 1 })).toBe('Drop 10 mm · 584 km · 1 épisode')
    expect(shoeHighlightDetail({ ...shoe, dropMm: null, painEpisodes: null })).toBe('584 km')
    expect(shoeHighlightDetail({ ...shoe, dropMm: null, distanceKm: null, painEpisodes: null })).toBe('')
  })
})
