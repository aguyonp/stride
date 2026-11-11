import { LANGS, useI18n } from '../../i18n'
import { cn } from '../../lib/cn'

/** Discreet `FR | EN` switch for screens without a profile (login, public share): localStorage only. */
export function LanguageToggle({ className }: { className?: string }) {
  const { lang, setLang, t } = useI18n()
  return (
    <div role="group" aria-label={t('common.lang.label')} className={cn('inline-flex items-center gap-0.5 text-xs font-semibold', className)}>
      {LANGS.map((l, i) => (
        <span key={l} className="inline-flex items-center">
          {i > 0 && <span className="px-0.5 text-ink-3/60" aria-hidden="true">|</span>}
          <button
            type="button"
            lang={l}
            aria-pressed={lang === l}
            aria-label={t(`common.lang.${l}`)}
            onClick={() => setLang(l)}
            className={cn('rounded-md px-1.5 py-1 uppercase transition-colors', lang === l ? 'text-accent' : 'text-ink-3 hover:text-ink')}
          >
            {l}
          </button>
        </span>
      ))}
    </div>
  )
}
