// Synthetic wellness per date (sleep, HRV, resting HR, Body Battery, steps, floors).

import type { DaySummary, RecoveryContext, TrendDay } from '../api/types'
import { addDays, diffDays, todayISO } from '../lib/format'
import { getCatalog } from './catalog'
import { hashStr, rng } from './sim'

interface RawDay {
  hrv: number
  sleepScore: number
  sleepS: number
  deepS: number
  remS: number
  awakeS: number
  rhr: number
  bbWake: number
  bbMin: number
  bbCharged: number
  bbDrained: number
  stress: number
  steps: number
  floorsUp: number
  floorsDown: number
  modMin: number
  vigMin: number
  activeCal: number
  bedtime: string
  wake: string
}

const BASELINE_LOW = 40
const BASELINE_HIGH = 55

function raw(date: string): RawDay {
  const r = rng(hashStr(`well-${date}`))
  const between = (a: number, b: number) => a + r() * (b - a)
  const sleepS = Math.round(between(6.4, 8.1) * 3600)
  const deepS = Math.round(between(62, 104) * 60)
  const remS = Math.round(between(75, 115) * 60)
  const awakeS = Math.round(between(6, 26) * 60)
  const bedH = 22 + Math.floor(between(0.6, 1.8))
  const bedM = Math.floor(between(0, 59))
  const base: RawDay = {
    hrv: Math.round(between(41, 52)),
    sleepScore: Math.round(between(66, 88)),
    sleepS,
    deepS,
    remS,
    awakeS,
    rhr: Math.round(between(45, 49)),
    bbWake: Math.round(between(58, 90)),
    bbMin: Math.round(between(18, 34)),
    bbCharged: Math.round(between(42, 70)),
    bbDrained: Math.round(between(45, 72)),
    stress: Math.round(between(22, 38)),
    steps: Math.round(between(6200, 13500)),
    floorsUp: Math.round(between(4, 17)),
    floorsDown: Math.round(between(4, 16)),
    modMin: Math.round(between(10, 45)),
    vigMin: Math.round(between(0, 25)),
    activeCal: Math.round(between(380, 900)),
    bedtime: `${String(bedH % 24).padStart(2, '0')}:${String(bedM).padStart(2, '0')}`,
    wake: '',
  }
  const wakeMin = (bedH * 60 + bedM + Math.round((sleepS + awakeS) / 60)) % (24 * 60)
  base.wake = `${String(Math.floor(wakeMin / 60)).padStart(2, '0')}:${String(wakeMin % 60).padStart(2, '0')}`

  const d = diffDays(date, todayISO())
  if (d === 0) Object.assign(base, { hrv: 37, sleepScore: 64, deepS: 52 * 60, rhr: 50, bbWake: 46, floorsUp: 24, floorsDown: 21, steps: 8432, modMin: 25, vigMin: 5, stress: 32, bbCharged: 45, bbDrained: 60 })
  if (d === 1) Object.assign(base, { hrv: 45, sleepScore: 78, sleepS: 7 * 3600 + 12 * 60, deepS: 82 * 60, rhr: 48, bbWake: 82, bedtime: '23:12', wake: '07:05', floorsUp: 9 })
  if (d === 2) Object.assign(base, { floorsUp: 18, floorsDown: 17, steps: 14_210, stress: 41 })
  if (d === 3) Object.assign(base, { hrv: 34, sleepScore: 56, rhr: 53, bbWake: 28 })
  if (d === 6) Object.assign(base, { hrv: 50, sleepScore: 85, rhr: 46, bbWake: 88, deepS: 95 * 60 })
  return base
}

const STATUS_LABEL: Record<string, string> = {
  BALANCED: 'Équilibré',
  UNBALANCED: 'Déséquilibré',
  LOW: 'Bas',
  POOR: 'Faible',
}

function hrvStatus(lastNight: number, weekly: number): string {
  if (lastNight < 32) return 'LOW'
  if (weekly < BASELINE_LOW || weekly > BASELINE_HIGH || lastNight < BASELINE_LOW - 2) return 'UNBALANCED'
  return 'BALANCED'
}

function sleepQualifier(score: number): string {
  if (score >= 90) return 'EXCELLENT'
  if (score >= 80) return 'GOOD'
  if (score >= 60) return 'FAIR'
  return 'POOR'
}

function loadOn(date: string): number | null {
  const items = getCatalog().filter((it) => it.date === date)
  if (!items.length) return null
  return items.reduce((s, it) => s + (it.base.trainingLoad ?? 0), 0)
}

function weeklyHrv(date: string): number {
  let s = 0
  for (let i = 0; i < 7; i++) s += raw(addDays(date, -i)).hrv
  return Math.round(s / 7)
}

function rhr7(date: string): number {
  let s = 0
  for (let i = 1; i <= 7; i++) s += raw(addDays(date, -i)).rhr
  return Math.round(s / 7)
}

export function trendDay(date: string): TrendDay {
  const r = raw(date)
  return {
    date,
    hrvMs: r.hrv,
    hrvStatus: hrvStatus(r.hrv, weeklyHrv(date)),
    sleepScore: r.sleepScore,
    sleepS: r.sleepS,
    deepS: r.deepS,
    restingHR: r.rhr,
    steps: r.steps,
    floorsUp: r.floorsUp,
    bodyBatteryMax: r.bbWake,
    trainingLoad: loadOn(date),
  }
}

/** Recovery context for the night ending on `date`. */
export function recoveryFor(date: string, opts: { missingSleep?: boolean } = {}): RecoveryContext {
  const r = raw(date)
  const weekly = weeklyHrv(date)
  const status = hrvStatus(r.hrv, weekly)
  const avg7 = rhr7(date)
  const lightS = Math.max(0, r.sleepS - r.deepS - r.remS)
  const trend: TrendDay[] = []
  for (let i = 6; i >= 0; i--) trend.push(trendDay(addDays(date, -i)))
  return {
    date,
    hrv: {
      lastNightAvgMs: r.hrv,
      lastNight5MinHighMs: r.hrv + 14,
      weeklyAvgMs: weekly,
      baselineLowMs: BASELINE_LOW,
      baselineHighMs: BASELINE_HIGH,
      status,
      statusLabel: STATUS_LABEL[status] ?? null,
      deltaVsWeeklyPct: Math.round(((r.hrv - weekly) / weekly) * 1000) / 10,
    },
    sleep: opts.missingSleep
      ? null
      : {
          score: r.sleepScore,
          qualifier: sleepQualifier(r.sleepScore),
          durationS: r.sleepS,
          deepS: r.deepS,
          remS: r.remS,
          lightS,
          awakeS: r.awakeS,
          avgSleepHR: r.rhr + 4,
          bedtimeLocal: r.bedtime,
          wakeLocal: r.wake,
        },
    restingHR: { bpm: r.rhr, avg7d: avg7, delta: r.rhr - avg7 },
    bodyBattery: { atWake: r.bbWake, max: r.bbWake + 2, min: r.bbMin, charged: r.bbCharged, drained: r.bbDrained },
    stressAvg: r.stress,
    trend,
  }
}

export function daySummary(date: string): DaySummary {
  const r = raw(date)
  return {
    steps: r.steps,
    stepGoal: 8000,
    floorsUp: r.floorsUp,
    floorsDown: r.floorsDown,
    intensityModerateMin: r.modMin,
    intensityVigorousMin: r.vigMin,
    activeCalories: r.activeCal,
    stressAvg: r.stress,
    bodyBatteryCharged: r.bbCharged,
    bodyBatteryDrained: r.bbDrained,
  }
}

export const HRV_STATUS_LABEL = STATUS_LABEL
