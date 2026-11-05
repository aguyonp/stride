import { Clock, CloudSun, Droplets, Flame, Gauge, HeartPulse, MessageSquareQuote, Mountain, Route, Thermometer, TrendingUp, Wind, Zap } from 'lucide-react'
import type { ActivityView } from '../../api/types'
import { fmtDateTime, fmtDuration, fmtNumber, fmtPaceValue, isNum } from '../../lib/format'
import { teLabel } from '../../lib/labels'
import { Card } from '../ui/Card'
import { NA, Stat, Value } from '../ui/Value'
import { garminReason } from './reason'

export function KpiGrid({ view, className }: { view: ActivityView; className?: string }) {
  const a = view.activity
  const gr = garminReason(view)
  return (
    <div className={className}>
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <Stat label="Distance" icon={<Route />} emphasis>
          <Value value={a.distanceM === null ? null : a.distanceM / 1000} format={(v) => fmtNumber(v, 2)} unit="km" />
        </Stat>
        <Stat label="Durée" icon={<Clock />} emphasis sub={a.movingDurationS && a.durationS && a.movingDurationS < a.durationS - 30 ? `en mouvement ${fmtDuration(a.movingDurationS)}` : undefined}>
          <Value value={a.durationS} format={fmtDuration} />
        </Stat>
        <Stat label="Allure moyenne" icon={<Gauge />} emphasis>
          <Value value={a.avgPaceSecPerKm} format={fmtPaceValue} unit="/km" />
        </Stat>
        <Stat label={<abbr title="Grade Adjusted Pace : allure équivalente sur le plat" className="no-underline">GAP (pente)</abbr>} icon={<Mountain />} emphasis sub={!a.sources.includes('garmin') && isNum(a.gapSecPerKm) ? 'calculé (modèle de pente)' : undefined}>
          <Value value={a.gapSecPerKm} format={fmtPaceValue} unit="/km" />
        </Stat>
        <Stat label="FC moyenne" icon={<HeartPulse />} sub={a.maxHR ? `max ${a.maxHR} bpm` : undefined}>
          <Value value={a.avgHR} format={(v) => fmtNumber(v)} unit="bpm" />
        </Stat>
        <Stat label="Puissance" icon={<Zap />} sub={a.normPower ? `NP ${fmtNumber(a.normPower)} W` : undefined}>
          <Value value={a.avgPower} format={(v) => fmtNumber(v)} unit="W" />
        </Stat>
        <Stat label="D+ / D−" icon={<TrendingUp />}>
          <span className="num">
            <Value value={a.elevGainM} format={(v) => `+${fmtNumber(v)}`} />
            <span className="mx-1 text-ink-3">/</span>
            <Value value={a.elevLossM} format={(v) => `−${fmtNumber(v)}`} unit="m" />
          </span>
        </Stat>
        <Stat label="Training Load" icon={<Flame />} sub={isNum(a.aerobicTE) ? `TE ${fmtNumber(a.aerobicTE, 1)}${teLabel(a.trainingEffectLabel) ? ` · ${teLabel(a.trainingEffectLabel)}` : ''}` : (gr ?? undefined)}>
          <Value value={a.trainingLoad} format={(v) => fmtNumber(v)} reason={null} />
        </Stat>
      </div>
    </div>
  )
}

const WEATHER_NA = 'Météo indisponible (pas de position GPS, séance en intérieur ou météo désactivée).'

export function WeatherCard({ view, className }: { view: ActivityView; className?: string }) {
  const w = view.weather
  return (
    <Card title="Météo au départ" icon={<CloudSun className="size-4" />} subtitle={w ? `${w.description || 'Conditions'} · ${w.source}` : undefined} className={className}>
      {!w ? (
        <p className="text-sm text-ink-3">
          <NA /> — {WEATHER_NA}
        </p>
      ) : (
        <>
          <div className="flex items-end gap-3">
            <p className="num text-4xl font-semibold tracking-tight">
              <Value value={w.temperatureC} format={(v) => fmtNumber(v, 1, { trim: true })} unit="°C" />
            </p>
            {isNum(w.apparentC) && <p className="num pb-1 text-sm text-ink-3">ressenti {fmtNumber(w.apparentC, 1, { trim: true })} °C</p>}
          </div>
          <dl className="num mt-3 grid grid-cols-3 gap-2 text-sm">
            <div className="glass-inset rounded-xl px-2.5 py-2">
              <dt className="flex items-center gap-1 text-[11px] text-ink-3">
                <Droplets className="size-3.5" aria-hidden="true" /> Humidité
              </dt>
              <dd className="font-semibold">
                <Value value={w.humidityPct} format={(v) => fmtNumber(v)} unit="%" />
              </dd>
            </div>
            <div className="glass-inset rounded-xl px-2.5 py-2">
              <dt className="flex items-center gap-1 text-[11px] text-ink-3">
                <Wind className="size-3.5" aria-hidden="true" /> Vent
              </dt>
              <dd className="truncate font-semibold">
                <Value value={w.windKph} format={(v) => fmtNumber(v)} unit="km/h" />
                {w.windDirLabel && <span className="ml-1 text-xs font-medium text-ink-3">{w.windDirLabel}</span>}
              </dd>
            </div>
            <div className="glass-inset rounded-xl px-2.5 py-2">
              <dt className="flex items-center gap-1 text-[11px] text-ink-3">
                <Thermometer className="size-3.5" aria-hidden="true" /> Pluie
              </dt>
              <dd className="font-semibold">
                <Value value={w.precipMm} format={(v) => fmtNumber(v, 1, { trim: true })} unit="mm" />
              </dd>
            </div>
          </dl>
          <p className="num mt-2 text-xs text-ink-3">
            {isNum(w.windGustKph) ? `Rafales ${fmtNumber(w.windGustKph)} km/h · ` : ''}Observé {fmtDateTime(w.observedAt)}
          </p>
        </>
      )}
    </Card>
  )
}

export function StravaCard({ view, className }: { view: ActivityView; className?: string }) {
  const s = view.activity.strava
  const linked = view.activity.sources.includes('strava')
  return (
    <Card title="Strava" icon={<MessageSquareQuote className="size-4" />} subtitle={s ? 'Titre, description, RPE et équipement' : undefined} className={className}>
      {!s ? (
        <p className="text-sm text-ink-3">
          <NA /> — {linked ? 'Détail Strava non disponible pour cette séance.' : 'Séance non liée à Strava (connecte Strava dans Réglages pour récupérer titre, RPE, Relative Effort et chaussures).'}
        </p>
      ) : (
        <div className="space-y-3">
          <div>
            <p className="font-semibold text-ink">{s.title || <NA />}</p>
            {s.description && <p className="mt-0.5 text-sm whitespace-pre-line text-ink-2">{s.description}</p>}
            {s.privateNote && <p className="mt-1.5 rounded-xl border border-line bg-track/50 px-2.5 py-1.5 text-xs text-ink-2">Note privée : {s.privateNote}</p>}
          </div>
          <dl className="num grid grid-cols-3 gap-2 text-sm">
            <div className="glass-inset rounded-xl px-2.5 py-2">
              <dt className="text-[11px] text-ink-3">RPE Strava</dt>
              <dd className="font-semibold">{isNum(s.rpe) ? `${s.rpe}/10` : <NA />}</dd>
            </div>
            <div className="glass-inset rounded-xl px-2.5 py-2">
              <dt className="text-[11px] text-ink-3">Relative Effort</dt>
              <dd className="font-semibold">
                <Value value={s.relativeEffort} format={(v) => fmtNumber(v)} />
              </dd>
            </div>
            <div className="glass-inset min-w-0 rounded-xl px-2.5 py-2">
              <dt className="text-[11px] text-ink-3">Équipement</dt>
              <dd className="truncate font-semibold">{s.gearName ?? <NA />}</dd>
            </div>
          </dl>
        </div>
      )}
    </Card>
  )
}
