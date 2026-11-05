import { useActivities, useActivityView } from '../../api/queries'
import { capitalize, fmtKm, fmtRelative } from '../../lib/format'
import { ui, useUi } from '../../state/ui'
import { Dialog } from '../ui/Dialog'
import { EmptyState, Spinner } from '../ui/Misc'
import { FeelingForm } from './FeelingForm'

/** Global bottom sheet opened from the mobile dock; targets the selected session or the latest run. */
export function FeelingSheet() {
  const open = useUi((s) => s.feelingOpen)
  const selected = useUi((s) => s.selectedActivityId)
  const list = useActivities({}, open && selected === null)
  const first = list.data?.pages[0]?.items ?? []
  const fallback = (first.find((a) => a.isRunning) ?? first[0])?.id ?? null
  const id = selected ?? fallback
  const view = useActivityView(open ? id : null)
  const v = view.data && view.data.activity.id === id ? view.data : null
  return (
    <Dialog
      open={open}
      onClose={ui.closeFeeling}
      title="Mon ressenti"
      description={v ? `${v.activity.name} · ${capitalize(fmtRelative(v.activity.startLocal))}${v.activity.distanceM ? ` · ${fmtKm(v.activity.distanceM)}` : ''}` : 'Dernière séance'}
    >
      {v ? (
        <FeelingForm activityId={v.activity.id} notes={v.notes} onSaved={ui.closeFeeling} stickyActions />
      ) : open && list.isSuccess && id === null ? (
        <EmptyState title="Aucune séance à commenter">Synchronise une activité Garmin ou Strava pour saisir ton ressenti.</EmptyState>
      ) : (
        <div className="grid min-h-40 place-items-center">
          <Spinner label="Chargement de la séance…" />
        </div>
      )}
    </Dialog>
  )
}
