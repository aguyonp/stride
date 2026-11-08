import type { KeyboardEvent } from 'react'
import type { BodySide, BodyZone, BodyZoneSummary } from '../../api/types'
import { cn } from '../../lib/cn'
import { fmtNumber } from '../../lib/format'
import { ALL_BODY_ZONES, BODY_SIDE_LABEL, BODY_ZONE_META, effectiveZoneScore, painScoreTone } from './bodyMapMeta'
import { t } from '../../i18n'

interface BodyAnatomySvgProps {
  zones: BodyZoneSummary[]
  selectedZone?: BodyZone | null
  onSelectZone?: (zone: BodyZone, sideHint?: BodySide) => void
  readOnly?: boolean
  className?: string
}

function sideDimmed(summary: BodyZoneSummary | undefined, side: 'left' | 'right'): boolean {
  if (!summary || !summary.latestSide || summary.latestSide === 'both') return false
  return summary.latestSide !== side
}

/**
 * Clinical inline SVG of the runner's lower body (anterior, posterior, and plantar sole views)
 * with keyboard-accessible interactive hotspots for Stride's canonical body zones.
 */
export function BodyAnatomySvg({
  zones,
  selectedZone = null,
  onSelectZone,
  readOnly = false,
  className,
}: BodyAnatomySvgProps) {
  const byZone = new Map<BodyZone, BodyZoneSummary>(zones.map((z) => [z.zone, z]))

  const zoneAttrs = (zone: BodyZone) => {
    const summary = byZone.get(zone)
    const meta = BODY_ZONE_META[zone]
    const score = effectiveZoneScore(summary)
    const tone = painScoreTone(score)
    const selected = selectedZone === zone
    const sideTxt = summary?.latestSide ? ` (${BODY_SIDE_LABEL[summary.latestSide].toLowerCase()})` : ''
    const ariaLabel =
      score > 0
        ? t('health.map.zoneAria', { label: meta.label, side: sideTxt, score: fmtNumber(score, 1, { trim: true }) })
        : t('health.map.zoneAriaNone', { label: meta.label })

    if (readOnly) {
      return {
        summary,
        tone,
        selected,
        groupProps: {
          'aria-label': ariaLabel,
          'data-zone': zone,
        },
      }
    }

    const onKeyDown = (e: KeyboardEvent<SVGGElement>) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        onSelectZone?.(zone)
      }
    }

    return {
      summary,
      tone,
      selected,
      groupProps: {
        role: 'button' as const,
        tabIndex: 0,
        'aria-pressed': selected,
        'aria-label': ariaLabel,
        'data-zone': zone,
        onClick: () => onSelectZone?.(zone),
        onKeyDown,
        className: 'cursor-pointer transition-opacity hover:opacity-95 focus:outline-none',
      },
    }
  }

  /** Click handler of one side of a zone (the group itself selects the zone without a side). */
  const pickSide = (zone: BodyZone, side: 'left' | 'right') => (e: { stopPropagation: () => void }) => {
    if (readOnly) return
    e.stopPropagation()
    onSelectZone?.(zone, side)
  }

  const shapeStyle = (zone: BodyZone, side: 'left' | 'right') => {
    const { summary, tone, selected } = zoneAttrs(zone)
    const dim = sideDimmed(summary, side)
    return {
      fill: dim ? 'color-mix(in oklab, var(--text-1) 4%, transparent)' : tone.svgFill,
      stroke: selected ? 'var(--accent)' : dim ? 'var(--line)' : tone.svgStroke,
      strokeWidth: selected ? 2.4 : 1.5,
    }
  }

  return (
    <div className={cn('select-none', className)}>
      <svg
        viewBox="0 0 560 430"
        className="mx-auto h-auto w-full max-w-[35rem]"
        role="img"
        aria-label={t('health.map.aria')}
      >
        {/* Column headers & L/R orientation */}
        <text x="140" y="22" textAnchor="middle" fill="var(--text-2)" fontSize="12" fontWeight="600">
          {t('health.map.front')}
        </text>
        <text x="82" y="42" textAnchor="middle" fill="var(--text-3)" fontSize="10" fontWeight="500">
          {t('common.misc.rightShort')}
        </text>
        <text x="198" y="42" textAnchor="middle" fill="var(--text-3)" fontSize="10" fontWeight="500">
          {t('common.misc.leftShort')}
        </text>

        <text x="420" y="22" textAnchor="middle" fill="var(--text-2)" fontSize="12" fontWeight="600">
          {t('health.map.back')}
        </text>
        <text x="362" y="42" textAnchor="middle" fill="var(--text-3)" fontSize="10" fontWeight="500">
          {t('common.misc.leftShort')}
        </text>
        <text x="478" y="42" textAnchor="middle" fill="var(--text-3)" fontSize="10" fontWeight="500">
          {t('common.misc.rightShort')}
        </text>

        {/* Center divider */}
        <line x1="280" y1="16" x2="280" y2="338" stroke="var(--line)" strokeDasharray="3 4" />

        {/* Anterior lower-body silhouette (pelvis + right leg at x~108 + left leg at x~172) */}
        <g fill="color-mix(in oklab, var(--text-1) 3.5%, transparent)" stroke="var(--line-strong)" strokeWidth="1.25">
          {/* Pelvis / waist */}
          <path d="M96 52 C112 46, 168 46, 184 52 L192 92 C176 102, 162 104, 140 100 C118 104, 104 102, 88 92 Z" />
          {/* Anatomical Right leg (viewer left) */}
          <path d="M90 92 C84 132, 87 168, 94 196 C91 214, 91 226, 95 242 C91 268, 95 304, 101 334 L94 352 C92 358, 98 362, 114 362 C124 362, 126 356, 122 346 L118 332 C123 302, 126 268, 122 242 C125 226, 125 212, 123 196 C129 166, 134 130, 134 98 Z" />
          {/* Anatomical Left leg (viewer right) */}
          <path d="M146 98 C146 130, 151 166, 157 196 C155 212, 155 226, 158 242 C154 268, 157 302, 162 332 L158 346 C154 356, 156 362, 166 362 C182 362, 188 358, 186 352 L179 334 C185 304, 189 268, 185 242 C189 226, 189 214, 186 196 C193 168, 196 132, 190 92 Z" />
        </g>

        {/* Posterior lower-body silhouette (pelvis + left leg at x~388 + right leg at x~452) */}
        <g fill="color-mix(in oklab, var(--text-1) 3.5%, transparent)" stroke="var(--line-strong)" strokeWidth="1.25">
          <path d="M376 52 C392 46, 448 46, 464 52 L472 94 C456 106, 438 106, 420 100 C402 106, 384 106, 368 94 Z" />
          {/* Anatomical Left leg (viewer left) */}
          <path d="M370 94 C364 132, 367 168, 374 196 C371 214, 371 226, 374 242 C368 268, 373 304, 381 334 L377 352 C376 359, 382 362, 395 362 C405 362, 408 357, 405 348 L401 332 C407 304, 410 268, 404 242 C406 226, 406 212, 404 196 C410 166, 414 130, 414 98 Z" />
          {/* Anatomical Right leg (viewer right) */}
          <path d="M426 98 C426 130, 430 166, 436 196 C434 212, 434 226, 436 242 C430 268, 433 304, 439 332 L435 348 C432 357, 435 362, 445 362 C458 362, 464 359, 463 352 L459 334 C467 304, 472 268, 466 242 C469 226, 469 214, 466 196 C473 168, 476 132, 470 94 Z" />
        </g>

        {/* Plantar callout box at bottom */}
        <rect
          x="154"
          y="372"
          width="252"
          height="48"
          rx="14"
          fill="color-mix(in oklab, var(--text-1) 3%, transparent)"
          stroke="var(--line)"
        />
        <text x="280" y="387" textAnchor="middle" fill="var(--text-3)" fontSize="10" fontWeight="600">
          {t('health.map.plantar')}
        </text>
        <text x="168" y="405" textAnchor="middle" fill="var(--text-3)" fontSize="9.5">
          D
        </text>
        <text x="392" y="405" textAnchor="middle" fill="var(--text-3)" fontSize="9.5">
          G
        </text>

        {/* 1. iliotibial_band: Lateral thigh & lateral knee (anterior view outer edges) */}
        <g {...zoneAttrs('iliotibial_band').groupProps}>
          <title>{BODY_ZONE_META.iliotibial_band.label}</title>
          {/* Right ITB (viewer left outer thigh/knee) */}
          <path
            d="M89 124 C85 154, 87 184, 92 214 L100 214 C96 184, 95 154, 98 124 Z"
            style={shapeStyle('iliotibial_band', 'right')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('iliotibial_band', 'right')
            }}
          />
          {/* Left ITB (viewer right outer thigh/knee) */}
          <path
            d="M191 124 C195 154, 193 184, 188 214 L180 214 C184 184, 185 154, 182 124 Z"
            style={shapeStyle('iliotibial_band', 'left')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('iliotibial_band', 'left')
            }}
          />
        </g>

        {/* 2. knee_pfps: Patella & peripatellar zone (anterior view) */}
        <g {...zoneAttrs('knee_pfps').groupProps}>
          <title>{BODY_ZONE_META.knee_pfps.label}</title>
          <ellipse
            cx="109"
            cy="212"
            rx="12"
            ry="15"
            style={shapeStyle('knee_pfps', 'right')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('knee_pfps', 'right')
            }}
          />
          <ellipse
            cx="171"
            cy="212"
            rx="12"
            ry="15"
            style={shapeStyle('knee_pfps', 'left')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('knee_pfps', 'left')
            }}
          />
        </g>

        {/* 3. shin_splints: Medial anterior tibia (anterior view) */}
        <g {...zoneAttrs('shin_splints').groupProps}>
          <title>{BODY_ZONE_META.shin_splints.label}</title>
          {/* Right medial tibia */}
          <path
            d="M112 248 C115 272, 116 298, 114 322 L121 322 C124 298, 123 272, 120 248 Z"
            style={shapeStyle('shin_splints', 'right')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('shin_splints', 'right')
            }}
          />
          {/* Left medial tibia */}
          <path
            d="M168 248 C165 272, 164 298, 166 322 L159 322 C156 298, 157 272, 160 248 Z"
            style={shapeStyle('shin_splints', 'left')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('shin_splints', 'left')
            }}
          />
        </g>

        {/* 4. hamstrings_calves: Posterior thigh & gastrocnemius (posterior view) */}
        <g {...zoneAttrs('hamstrings_calves').groupProps}>
          <title>{BODY_ZONE_META.hamstrings_calves.label}</title>
          {/* Left hamstring + calf (viewer left on posterior) */}
          <g
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('hamstrings_calves', 'left')
            }}
          >
            <ellipse cx="390" cy="148" rx="14" ry="32" style={shapeStyle('hamstrings_calves', 'left')} />
            <ellipse cx="389" cy="268" rx="13" ry="26" style={shapeStyle('hamstrings_calves', 'left')} />
          </g>
          {/* Right hamstring + calf (viewer right on posterior) */}
          <g
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('hamstrings_calves', 'right')
            }}
          >
            <ellipse cx="450" cy="148" rx="14" ry="32" style={shapeStyle('hamstrings_calves', 'right')} />
            <ellipse cx="451" cy="268" rx="13" ry="26" style={shapeStyle('hamstrings_calves', 'right')} />
          </g>
        </g>

        {/* 5. achilles: Distal posterior tendon (posterior view) */}
        <g {...zoneAttrs('achilles').groupProps}>
          <title>{BODY_ZONE_META.achilles.label}</title>
          {/* Left Achilles */}
          <rect
            x="384"
            y="306"
            width="11"
            height="36"
            rx="5.5"
            style={shapeStyle('achilles', 'left')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('achilles', 'left')
            }}
          />
          {/* Right Achilles */}
          <rect
            x="445"
            y="306"
            width="11"
            height="36"
            rx="5.5"
            style={shapeStyle('achilles', 'right')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('achilles', 'right')
            }}
          />
        </g>

        {/* 6. plantar_fascia: Plantar heel & arch callout */}
        <g {...zoneAttrs('plantar_fascia').groupProps}>
          <title>{BODY_ZONE_META.plantar_fascia.label}</title>
          {/* Right plantar sole */}
          <ellipse
            cx="236"
            cy="402"
            rx="28"
            ry="10"
            style={shapeStyle('plantar_fascia', 'right')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('plantar_fascia', 'right')
            }}
          />
          {/* Left plantar sole */}
          <ellipse
            cx="324"
            cy="402"
            rx="28"
            ry="10"
            style={shapeStyle('plantar_fascia', 'left')}
            onClick={(e) => {
              if (readOnly) return
              e.stopPropagation()
              onSelectZone?.('plantar_fascia', 'left')
            }}
          />
        </g>

        {/* 7. ankle: Malleoli & ankle ligaments (anterior view, above the foot) */}
        <g {...zoneAttrs('ankle').groupProps}>
          <title>{BODY_ZONE_META.ankle.label}</title>
          <ellipse cx="110" cy="338" rx="11" ry="6" style={shapeStyle('ankle', 'right')} onClick={pickSide('ankle', 'right')} />
          <ellipse cx="170" cy="338" rx="11" ry="6" style={shapeStyle('ankle', 'left')} onClick={pickSide('ankle', 'left')} />
        </g>

        {/* 8. forefoot: Metatarsal pads, in front of the plantar heel & arch */}
        <g {...zoneAttrs('forefoot').groupProps}>
          <title>{BODY_ZONE_META.forefoot.label}</title>
          <ellipse cx="194" cy="402" rx="13" ry="9" style={shapeStyle('forefoot', 'right')} onClick={pickSide('forefoot', 'right')} />
          <ellipse cx="366" cy="402" rx="13" ry="9" style={shapeStyle('forefoot', 'left')} onClick={pickSide('forefoot', 'left')} />
        </g>

        {/* 9. hip: Lateral hip (greater trochanter) & groin, top of the thigh (anterior view) */}
        <g {...zoneAttrs('hip').groupProps}>
          <title>{BODY_ZONE_META.hip.label}</title>
          <ellipse cx="101" cy="102" rx="10" ry="12" style={shapeStyle('hip', 'right')} onClick={pickSide('hip', 'right')} />
          <ellipse cx="179" cy="102" rx="10" ry="12" style={shapeStyle('hip', 'left')} onClick={pickSide('hip', 'left')} />
        </g>

        {/* 10. lower_back: Lumbar paraspinal area above the pelvis (posterior view) */}
        <g {...zoneAttrs('lower_back').groupProps}>
          <title>{BODY_ZONE_META.lower_back.label}</title>
          <rect x="400" y="56" width="15" height="30" rx="7" style={shapeStyle('lower_back', 'left')} onClick={pickSide('lower_back', 'left')} />
          <rect x="425" y="56" width="15" height="30" rx="7" style={shapeStyle('lower_back', 'right')} onClick={pickSide('lower_back', 'right')} />
        </g>
      </svg>

      {/* Compact legend + quick zone buttons */}
      <div className="mt-3 flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5 text-[11px] text-ink-3">
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-full border border-line-strong bg-track" aria-hidden="true" /> {t('health.map.none')}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-full bg-ok" aria-hidden="true" /> {t('health.map.mild')}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-full bg-warn" aria-hidden="true" /> {t('health.map.moderate')}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-full bg-danger" aria-hidden="true" /> {t('health.map.severe')}
        </span>
      </div>

      {!readOnly && onSelectZone && (
        <div className="mt-3 flex flex-wrap justify-center gap-1.5" aria-label={t('health.map.quickSelect')}>
          {ALL_BODY_ZONES.map((z) => {
            const summary = byZone.get(z)
            const score = effectiveZoneScore(summary)
            const tone = painScoreTone(score)
            const active = selectedZone === z
            return (
              <button
                key={z}
                type="button"
                onClick={() => onSelectZone(z)}
                aria-pressed={active}
                className={cn(
                  'chip h-7 border px-2.5 text-xs transition-colors',
                  active ? 'border-accent bg-accent/15 text-ink' : 'border-line bg-track/50 text-ink-2 hover:bg-track',
                )}
              >
                <span
                  className={cn(
                    'size-2 rounded-full',
                    tone.level === 'danger'
                      ? 'bg-danger'
                      : tone.level === 'warn'
                        ? 'bg-warn'
                        : tone.level === 'low'
                          ? 'bg-ok'
                          : 'bg-line-strong',
                  )}
                  aria-hidden="true"
                />
                <span>{BODY_ZONE_META[z].short}</span>
                {score > 0 && <span className={cn('num font-semibold', tone.textCls)}>{fmtNumber(score, 1, { trim: true })}</span>}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
