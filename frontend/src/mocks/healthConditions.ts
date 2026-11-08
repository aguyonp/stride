// Mock copy of the backend pathology catalog (§14.7, backend/internal/analysis/conditions.go).
// The app reads the catalog from GET /api/health/conditions; only the mock server uses this file.

import type { AthleteProfile, BodyZone, HealthCondition, HealthConditionCatalog, HealthZone } from '../api/types'
import { ALL_BODY_ZONES, BODY_ZONE_META } from '../components/bodymap/bodyMapMeta'
import { getLang, type Lang } from '../i18n/lang'

type MockCondition = HealthCondition & { preset?: AthleteProfile['focusArea'] }

const OTHER: MockCondition = { key: 'other', label: 'Autre (préciser)' }

const BY_ZONE: Record<BodyZone, MockCondition[]> = {
  knee_pfps: [
    { key: 'patellofemoral_pain', label: 'Syndrome fémoro-patellaire', preset: 'knee_pfps' },
    { key: 'patellar_tendinopathy', label: 'Tendinopathie rotulienne' },
    { key: 'pes_anserine_tendinopathy', label: 'Tendinopathie de la patte d’oie' },
    { key: 'meniscus_tear', label: 'Lésion méniscale' },
    { key: 'knee_ligament_sprain', label: 'Entorse du genou (ligament)' },
  ],
  shin_splints: [
    { key: 'medial_tibial_stress', label: 'Périostite tibiale', preset: 'shin_splints' },
    { key: 'tibial_stress_fracture', label: 'Fracture de fatigue du tibia' },
    { key: 'compartment_syndrome', label: 'Syndrome des loges' },
  ],
  iliotibial_band: [{ key: 'iliotibial_band_syndrome', label: 'Syndrome de la bandelette ilio-tibiale' }],
  achilles: [
    { key: 'achilles_tendinopathy', label: 'Tendinopathie d’Achille', preset: 'achilles' },
    { key: 'retrocalcaneal_bursitis', label: 'Bursite rétro-calcanéenne' },
    { key: 'achilles_partial_tear', label: 'Rupture partielle du tendon d’Achille' },
  ],
  plantar_fascia: [
    { key: 'plantar_fasciitis', label: 'Aponévrosite plantaire' },
    { key: 'calcaneal_stress_fracture', label: 'Fracture de fatigue du calcanéum' },
  ],
  hamstrings_calves: [
    { key: 'hamstring_strain', label: 'Lésion musculaire des ischio-jambiers' },
    { key: 'calf_strain', label: 'Lésion musculaire du mollet' },
    { key: 'proximal_hamstring_tendinopathy', label: 'Tendinopathie proximale des ischio-jambiers' },
  ],
  ankle: [
    { key: 'ankle_sprain', label: 'Entorse de cheville' },
    { key: 'ankle_fracture', label: 'Fracture de cheville (malléole)' },
    { key: 'peroneal_tendinopathy', label: 'Tendinopathie des fibulaires' },
    { key: 'tibialis_posterior_tendinopathy', label: 'Tendinopathie du tibial postérieur' },
    { key: 'chronic_ankle_instability', label: 'Instabilité chronique de cheville' },
  ],
  forefoot: [
    { key: 'metatarsal_stress_fracture', label: 'Fracture de fatigue métatarsienne' },
    { key: 'metatarsalgia', label: 'Métatarsalgie' },
    { key: 'morton_neuroma', label: 'Névrome de Morton' },
  ],
  hip: [
    { key: 'gluteal_tendinopathy', label: 'Tendinopathie fessière (moyen fessier)' },
    { key: 'hip_flexor_tendinopathy', label: 'Tendinopathie du psoas' },
    { key: 'pubalgia', label: 'Pubalgie' },
    { key: 'femoral_neck_stress_fracture', label: 'Fracture de fatigue du col du fémur' },
  ],
  lower_back: [
    { key: 'low_back_pain', label: 'Lombalgie' },
    { key: 'sciatica', label: 'Sciatique' },
    { key: 'sacroiliac_pain', label: 'Douleur sacro-iliaque' },
  ],
}

/** English labels of the catalog (§15.2: the backend localizes them from `Accept-Language`). */
const EN_LABEL: Record<string, string> = {
  other: 'Other (specify)',
  patellofemoral_pain: 'Patellofemoral pain syndrome',
  patellar_tendinopathy: 'Patellar tendinopathy',
  pes_anserine_tendinopathy: 'Pes anserine tendinopathy',
  meniscus_tear: 'Meniscus tear',
  knee_ligament_sprain: 'Knee sprain (ligament)',
  medial_tibial_stress: 'Medial tibial stress syndrome (shin splints)',
  tibial_stress_fracture: 'Tibial stress fracture',
  compartment_syndrome: 'Compartment syndrome',
  iliotibial_band_syndrome: 'Iliotibial band syndrome',
  achilles_tendinopathy: 'Achilles tendinopathy',
  retrocalcaneal_bursitis: 'Retrocalcaneal bursitis',
  achilles_partial_tear: 'Partial Achilles tendon tear',
  plantar_fasciitis: 'Plantar fasciitis',
  calcaneal_stress_fracture: 'Calcaneal stress fracture',
  hamstring_strain: 'Hamstring strain',
  calf_strain: 'Calf strain',
  proximal_hamstring_tendinopathy: 'Proximal hamstring tendinopathy',
  ankle_sprain: 'Ankle sprain',
  ankle_fracture: 'Ankle fracture (malleolus)',
  peroneal_tendinopathy: 'Peroneal tendinopathy',
  tibialis_posterior_tendinopathy: 'Tibialis posterior tendinopathy',
  chronic_ankle_instability: 'Chronic ankle instability',
  metatarsal_stress_fracture: 'Metatarsal stress fracture',
  metatarsalgia: 'Metatarsalgia',
  morton_neuroma: 'Morton’s neuroma',
  gluteal_tendinopathy: 'Gluteal tendinopathy (gluteus medius)',
  hip_flexor_tendinopathy: 'Hip flexor (psoas) tendinopathy',
  pubalgia: 'Athletic pubalgia',
  femoral_neck_stress_fracture: 'Femoral neck stress fracture',
  low_back_pain: 'Low back pain',
  sciatica: 'Sciatica',
  sacroiliac_pain: 'Sacroiliac joint pain',
}

const labelIn = (c: HealthCondition, lang: Lang) => (lang === 'en' ? (EN_LABEL[c.key] ?? c.label) : c.label)

/** GET /api/health/conditions in the request language (`Accept-Language`, fr or en). */
export function conditionCatalog(lang: Lang = getLang()): HealthConditionCatalog {
  return {
    zones: ALL_BODY_ZONES.map((zone) => ({
      zone,
      zoneLabel: BODY_ZONE_META[zone].label,
      conditions: [...BY_ZONE[zone], OTHER].map((c) => ({ key: c.key, label: labelIn(c, lang) })),
    })),
  }
}

const find = (zone: BodyZone, key: string | null | undefined): MockCondition | undefined => (key === OTHER.key ? OTHER : BY_ZONE[zone].find((c) => c.key === key))

/** Same rules as the backend: catalog key of the zone, or « other » with 1–80 printable characters. */
export function normalizeCondition(z: HealthZone): { zone: HealthZone } | { error: string } {
  const condition = (z.condition ?? '').trim()
  const base: HealthZone = { zone: z.zone, side: z.side, since: z.since ?? null, note: (z.note ?? '').trim() }
  if (!condition) return { zone: base }
  if (!find(z.zone, condition)) return { error: `Pathologie inconnue pour la zone ${BODY_ZONE_META[z.zone].label}.` }
  if (condition !== OTHER.key) return { zone: { ...base, condition } }
  const customLabel = (z.customLabel ?? '').trim()
  if (!customLabel || customLabel.length > 80 || /[\p{Cc}\p{Cf}]/u.test(customLabel)) return { error: 'Précisez la pathologie (1 à 80 caractères, texte simple).' }
  return { zone: { ...base, condition, customLabel } }
}

export function conditionLabelOf(z: HealthZone, lang: Lang = getLang()): string | null {
  if (z.condition === OTHER.key) return z.customLabel ?? null
  const c = find(z.zone, z.condition)
  return c ? labelIn(c, lang) : null
}

/** Dedicated preset of a condition, `other` for any other condition, null without condition. */
export function conditionPreset(z: HealthZone): AthleteProfile['focusArea'] | null {
  return z.condition ? (find(z.zone, z.condition)?.preset ?? 'other') : null
}
