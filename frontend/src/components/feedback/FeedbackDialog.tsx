import { Bug, Lightbulb, Loader2, MessageCircle, Send } from 'lucide-react'
import { useState } from 'react'
import { useSendFeedback, useVersion } from '../../api/queries'
import type { FeedbackCategory, FeedbackContext } from '../../api/types'
import { Dialog } from '../ui/Dialog'
import { Checkbox, Field } from '../ui/Field'
import { InlineError } from '../ui/Misc'
import { SegmentedControl } from '../ui/SegmentedControl'
import { useToast } from '../ui/Toast'
import { t } from '../../i18n'

export const FEEDBACK_MIN = 5
export const FEEDBACK_MAX = 4000
const EMAIL_KEY = 'stride.feedback.email'
const EMAIL_RE = /^[^\s@<>"]+@[^\s@<>"]+\.[^\s@<>"]+$/

/** The last address the user agreed to be contacted at, on this device only. */
function rememberedEmail(): string {
  try {
    return window.localStorage.getItem(EMAIL_KEY) ?? ''
  } catch {
    return ''
  }
}

function remember(email: string) {
  try {
    window.localStorage.setItem(EMAIL_KEY, email)
  } catch {
    /* storage unavailable: nothing to remember */
  }
}

/** Non-sensitive context: path only (query strings may carry invite codes), screen size, browser. */
export function feedbackContext(version: string): FeedbackContext {
  return {
    appVersion: version,
    route: window.location.pathname,
    viewport: `${window.innerWidth}×${window.innerHeight}`,
    userAgent: navigator.userAgent,
  }
}

export function FeedbackDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const send = useSendFeedback()
  const version = useVersion()
  const { toast } = useToast()
  const [category, setCategory] = useState<FeedbackCategory>('bug')
  const [message, setMessage] = useState('')
  const [contactOk, setContactOk] = useState(false)
  const [email, setEmail] = useState(rememberedEmail)
  const [error, setError] = useState<string | null>(null)

  const length = message.trim().length
  const emailOk = !contactOk || EMAIL_RE.test(email.trim())
  const valid = length >= FEEDBACK_MIN && length <= FEEDBACK_MAX && emailOk
  const ctx = feedbackContext(version.data?.version ?? 'inconnue')

  const close = () => {
    setError(null)
    onClose()
  }
  const submit = () => {
    if (!valid || send.isPending) return
    setError(null)
    const contactEmail = email.trim()
    send.mutate(
      { category, message: message.trim(), contactOk, ...(contactOk ? { contactEmail } : {}), context: ctx },
      {
        onSuccess: () => {
          if (contactOk) remember(contactEmail)
          toast({ title: t('coach.feedback.thanks'), description: contactOk ? t('coach.feedback.thanksContact') : t('coach.feedback.thanksSent') })
          setMessage('')
          setContactOk(false)
          close()
        },
        onError: (e) => setError(e.message),
      },
    )
  }

  return (
    <Dialog
      open={open}
      onClose={close}
      title={t('coach.feedback.title')}
      description={t('coach.feedback.description')}
      initialFocus="textarea"
      footer={
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-ghost" onClick={close}>
            {t('common.actions.cancel')}
          </button>
          <button type="button" className="btn btn-primary" disabled={!valid || send.isPending} onClick={submit}>
            {send.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Send className="size-4" aria-hidden="true" />}
            {t('coach.feedback.send')}
          </button>
        </div>
      }
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <SegmentedControl<FeedbackCategory>
          ariaLabel={t('coach.feedback.type')}
          value={category}
          onChange={setCategory}
          options={[
            { value: 'bug', label: t('coach.feedback.bug'), icon: <Bug className="size-4" /> },
            { value: 'idea', label: t('coach.feedback.idea'), icon: <Lightbulb className="size-4" /> },
            { value: 'other', label: t('coach.feedback.other'), icon: <MessageCircle className="size-4" /> },
          ]}
        />
        <Field
          label={t('coach.feedback.message')}
          hint={
            <span className="num">
              {category === 'bug' ? t('coach.feedback.bugHint') : ''}
              {length} / {FEEDBACK_MAX}
            </span>
          }
          error={length > FEEDBACK_MAX ? t('coach.feedback.tooLong', { max: FEEDBACK_MAX }) : undefined}
        >
          {(p) => (
            <textarea
              {...p}
              className="field min-h-32 resize-y text-sm"
              required
              maxLength={FEEDBACK_MAX + 200}
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              placeholder={category === 'idea' ? t('coach.feedback.ideaPlaceholder') : category === 'bug' ? t('coach.feedback.bugPlaceholder') : t('coach.feedback.otherPlaceholder')}
            />
          )}
        </Field>
        <Checkbox checked={contactOk} onChange={setContactOk} label={t('coach.feedback.contact')} description={t('coach.feedback.contactHint')} />
        {contactOk && (
          <Field label={t('coach.feedback.email')} error={email.trim() && !emailOk ? t('coach.feedback.emailInvalid') : undefined}>
            {(p) => <input {...p} type="email" autoComplete="email" inputMode="email" className="field text-sm" maxLength={254} value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t('coach.feedback.emailPlaceholder')} />}
          </Field>
        )}
        <details className="rounded-xl border border-line px-3 py-2 text-xs text-ink-3">
          <summary className="cursor-pointer font-medium text-ink-2">{t('coach.feedback.attached')}</summary>
          <dl className="mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1">
            <dt>{t('coach.feedback.version')}</dt>
            <dd className="num text-ink-2">{ctx.appVersion}</dd>
            <dt>{t('coach.feedback.page')}</dt>
            <dd className="break-all text-ink-2">{ctx.route}</dd>
            <dt>{t('coach.feedback.screen')}</dt>
            <dd className="num text-ink-2">{ctx.viewport}</dd>
            <dt>{t('coach.feedback.browser')}</dt>
            <dd className="break-all text-ink-2">{ctx.userAgent}</dd>
          </dl>
        </details>
        {error && <InlineError>{error}</InlineError>}
      </form>
    </Dialog>
  )
}
