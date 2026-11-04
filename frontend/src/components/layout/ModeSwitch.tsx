import { Footprints, HeartPulse, Zap } from 'lucide-react'
import type { Mode } from '../../api/types'
import { SegmentedControl } from '../ui/SegmentedControl'

export function ModeSwitch({ mode, onChange, className }: { mode: Mode; onChange: (m: Mode) => void; className?: string }) {
  return (
    <SegmentedControl<Mode>
      ariaLabel="Mode d'analyse"
      className={className}
      value={mode}
      onChange={onChange}
      options={[
        {
          value: 'sfp',
          icon: <Footprints />,
          ariaLabel: 'Rééducation SFP',
          label: (
            <>
              <span className="sm:hidden">Rééduc</span>
              <span className="hidden sm:inline">Rééducation SFP</span>
            </>
          ),
        },
        {
          value: 'recovery',
          icon: <HeartPulse />,
          ariaLabel: 'Santé et récupération',
          label: (
            <>
              <span className="sm:hidden">Récup</span>
              <span className="hidden sm:inline">Santé & Récup</span>
            </>
          ),
        },
        {
          value: 'performance',
          icon: <Zap />,
          ariaLabel: 'Performance',
          label: (
            <>
              <span className="sm:hidden">Perf</span>
              <span className="hidden sm:inline">Performance</span>
            </>
          ),
        },
      ]}
    />
  )
}
