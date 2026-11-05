import { FileUp, Loader2, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Link } from 'wouter'
import { useImportFile } from '../../api/queriesV3'
import { InlineError } from '../../components/ui/Misc'
import { Section } from './Section'

const MAX = 25 * 1024 * 1024

/** Discreet rescue import of a .fit / .gpx / .tcx file (deduplicated server-side). */
export function ImportSection() {
  const imp = useImportFile()
  const input = useRef<HTMLInputElement>(null)
  const [tooBig, setTooBig] = useState(false)
  const res = imp.data
  return (
    <Section id="import" title="Import de secours" icon={<FileUp />} description="Quand une synchro échoue : importe le fichier exporté de ta montre ou d’une autre app. Les doublons sont fusionnés automatiquement.">
      <div className="flex flex-wrap items-center gap-3">
        <input
          ref={input}
          type="file"
          accept=".fit,.gpx,.tcx"
          className="sr-only"
          id="rescue-file"
          onChange={(e) => {
            const f = e.target.files?.[0]
            e.target.value = ''
            if (!f) return
            imp.reset()
            setTooBig(f.size > MAX)
            if (f.size <= MAX) imp.mutate(f)
          }}
        />
        <button type="button" className="btn btn-ghost" onClick={() => input.current?.click()} disabled={imp.isPending}>
          {imp.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Upload className="size-4" aria-hidden="true" />}
          Choisir un fichier
        </button>
        <span className="text-xs text-ink-3">.fit, .gpx ou .tcx · 25 Mo max</span>
      </div>
      <div aria-live="polite" className="mt-3">
        {tooBig && <InlineError>Fichier trop volumineux (25 Mo max).</InlineError>}
        {imp.isError && <InlineError>{imp.error.message}</InlineError>}
        {res && (
          <p className="text-sm text-ink-2">
            {res.merged ? 'Fusionné avec une séance existante.' : 'Séance importée.'}{' '}
            <Link href={`/?s=${res.activityId}`} className="font-semibold text-accent underline underline-offset-2">
              Voir la séance
            </Link>
          </p>
        )}
      </div>
    </Section>
  )
}
