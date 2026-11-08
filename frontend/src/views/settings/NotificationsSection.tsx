import { Bell, CircleCheck, CircleX, Loader2, Plus, Send, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useNotificationMutations, useNotifications } from '../../api/queriesV3'
import type { NotificationInput, NotificationKind, NotificationTarget } from '../../api/types'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Dialog } from '../../components/ui/Dialog'
import { Checkbox, Field } from '../../components/ui/Field'
import { InlineError, Spinner } from '../../components/ui/Misc'
import { useToast } from '../../components/ui/Toast'
import { fmtDateTime } from '../../lib/format'
import { Section } from './Section'
import { lazyRecord, t } from '../../i18n'

const KIND_LABEL: Record<NotificationKind, string> = lazyRecord(['discord', 'telegram', 'json'] as const, (k) => (k === 'json' ? t('settings.notifications.jsonWebhook') : k === 'discord' ? 'Discord' : 'Telegram'))

function NotificationDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const m = useNotificationMutations()
  const [f, setF] = useState<NotificationInput>({ kind: 'discord', label: '', enabled: true, url: '', botToken: '', chatId: '' })
  const set = <K extends keyof NotificationInput>(k: K, v: NotificationInput[K]) => setF({ ...f, [k]: v })
  const submit = () => {
    const body: NotificationInput = { kind: f.kind, label: f.label.trim() || KIND_LABEL[f.kind], enabled: f.enabled }
    if (f.kind === 'telegram') Object.assign(body, { botToken: f.botToken?.trim(), chatId: f.chatId?.trim() })
    else body.url = f.url?.trim()
    m.create.mutate(body, { onSuccess: onClose })
  }
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t('settings.notifications.newTitle')}
      description={t('settings.notifications.newHint')}
      footer={
        <div className="flex justify-end gap-2 pb-3">
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            {t('common.actions.cancel')}
          </button>
          <button type="submit" form="notif-form" className="btn btn-primary" disabled={m.create.isPending}>
            {m.create.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {t('common.actions.add')}
          </button>
        </div>
      }
    >
      <form
        id="notif-form"
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <Field label={t('settings.notifications.type')}>
          {(p) => (
            <select {...p} className="field" value={f.kind} onChange={(e) => set('kind', e.target.value as NotificationKind)}>
              <option value="discord">{t('settings.notifications.discordOption')}</option>
              <option value="telegram">{t('settings.notifications.telegramOption')}</option>
              <option value="json">{t('settings.notifications.jsonWebhook')}</option>
            </select>
          )}
        </Field>
        <Field label={t('settings.notifications.name')}>{(p) => <input {...p} className="field" value={f.label} placeholder={KIND_LABEL[f.kind]} onChange={(e) => set('label', e.target.value)} maxLength={60} />}</Field>
        {f.kind === 'telegram' ? (
          <>
            <Field label={t('settings.notifications.botToken')} hint={t('settings.notifications.botTokenHint')}>
              {(p) => <input {...p} className="field" type="password" autoComplete="off" value={f.botToken} onChange={(e) => set('botToken', e.target.value)} required />}
            </Field>
            <Field label="Chat ID">{(p) => <input {...p} className="field" inputMode="numeric" value={f.chatId} onChange={(e) => set('chatId', e.target.value)} required />}</Field>
          </>
        ) : (
          <Field label="URL" hint={f.kind === 'discord' ? t('settings.notifications.discordHint') : t('settings.notifications.jsonHint', { payload: '{event: "activity.synced", …}' })}>
            {(p) => <input {...p} className="field" type="url" inputMode="url" autoComplete="off" placeholder="https://…" value={f.url} onChange={(e) => set('url', e.target.value)} required />}
          </Field>
        )}
        <Checkbox checked={f.enabled} onChange={(v) => set('enabled', v)} label={t('settings.notifications.enable')} />
        {m.create.isError && <InlineError>{m.create.error.message}</InlineError>}
      </form>
    </Dialog>
  )
}

function Row({ n }: { n: NotificationTarget }) {
  const m = useNotificationMutations()
  const { toast } = useToast()
  const [confirm, setConfirm] = useState(false)
  const ok = n.lastError === null && n.lastStatus !== null
  return (
    <li className="glass-inset flex flex-wrap items-center gap-3 p-3.5">
      <div className="min-w-0 flex-1 basis-52">
        <p className="font-semibold text-ink">
          {n.label} <span className="text-xs font-normal text-ink-3">· {KIND_LABEL[n.kind]}</span>
        </p>
        <p className="num truncate text-xs text-ink-3">{n.hint}</p>
        <p className="mt-1 flex items-center gap-1.5 text-xs">
          {n.lastSentAt === null ? (
            <span className="text-ink-3">{t('settings.notifications.neverSent')}</span>
          ) : ok ? (
            <span className="flex items-center gap-1 text-ok">
              <CircleCheck className="size-3.5" aria-hidden="true" /> {t('settings.notifications.lastSent', { date: fmtDateTime(n.lastSentAt), status: n.lastStatus })}
            </span>
          ) : (
            <span className="flex items-center gap-1 text-danger">
              <CircleX className="size-3.5" aria-hidden="true" /> {n.lastError ?? t('settings.notifications.failed', { status: n.lastStatus })} · {fmtDateTime(n.lastSentAt)}
            </span>
          )}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <Checkbox checked={n.enabled} onChange={(enabled) => m.update.mutate({ id: n.id, enabled })} label={t('settings.notifications.active')} disabled={m.update.isPending} />
        <button
          type="button"
          className="btn btn-ghost h-9 px-3"
          disabled={m.test.isPending}
          onClick={() =>
            m.test.mutate(n.id, {
              onSuccess: (r) => toast(r.ok ? { title: t('settings.notifications.testSent'), description: r.message, tone: 'success' } : { title: t('settings.notifications.testFailed'), description: r.message, tone: 'error' }),
              onError: (e) => toast({ title: t('settings.notifications.testFailed'), description: e.message, tone: 'error' }),
            })
          }
        >
          {m.test.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Send className="size-4" aria-hidden="true" />}
          {t('common.actions.test')}
        </button>
        <button type="button" className="icon-btn size-9" aria-label={t('settings.notifications.deleteLabel', { name: n.label })} onClick={() => setConfirm(true)}>
          <Trash2 className="size-4" />
        </button>
      </div>
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title={t('settings.notifications.deleteTitle')}
        message={t('settings.notifications.deleteBody', { name: n.label })}
        confirmLabel={t('common.actions.delete')}
        danger
        busy={m.remove.isPending}
        onConfirm={() => m.remove.mutate(n.id, { onSuccess: () => setConfirm(false) })}
      />
    </li>
  )
}

/** Settings › Notifications: Discord / Telegram / JSON webhook targets, enable, test, last status. */
export function NotificationsSection() {
  const q = useNotifications()
  const [adding, setAdding] = useState(false)
  return (
    <Section
      id="notifications"
      title={t('settings.notifications.title')}
      icon={<Bell />}
      description={t('settings.notifications.description')}
      actions={
        <button type="button" className="btn btn-ghost" onClick={() => setAdding(true)}>
          <Plus className="size-4" aria-hidden="true" /> {t('common.actions.add')}
        </button>
      }
    >
      {q.isPending ? (
        <Spinner label={t('common.actions.loading')} />
      ) : q.isError ? (
        <InlineError>{q.error.message}</InlineError>
      ) : q.data.length === 0 ? (
        <p className="text-sm text-ink-3">{t('settings.notifications.empty')}</p>
      ) : (
        <ul className="space-y-2">
          {q.data.map((n) => (
            <Row key={n.id} n={n} />
          ))}
        </ul>
      )}
      {adding && <NotificationDialog open={adding} onClose={() => setAdding(false)} />}
    </Section>
  )
}
