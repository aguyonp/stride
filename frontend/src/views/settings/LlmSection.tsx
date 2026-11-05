import { Bot, CircleCheck, CircleX, Loader2, PlugZap, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useLlmConfig, useLlmMutations } from '../../api/queriesV3'
import type { LLMConfigInput, LLMProvider } from '../../api/types'
import { PROVIDER_LABEL } from '../../components/ai/AnalysisDrawer'
import { ConfirmDialog } from '../../components/ui/ConfirmDialog'
import { Field } from '../../components/ui/Field'
import { InlineError, Spinner } from '../../components/ui/Misc'
import { useToast } from '../../components/ui/Toast'
import { fmtDateTime } from '../../lib/format'
import { Section } from './Section'

const DEFAULT_MODEL: Record<LLMProvider, string> = { gemini: 'gemini-2.5-flash', anthropic: 'claude-sonnet-4-5', openai: 'gpt-4.1-mini', openai_compat: 'llama3.1:8b' }

/** Settings › IA: provider, model, write-only API key (hint only), test. */
export function LlmSection() {
  const cfg = useLlmConfig()
  const m = useLlmMutations()
  const { toast } = useToast()
  const c = cfg.data
  const [draft, setDraft] = useState<{ provider: LLMProvider; model: string; baseUrl: string; apiKey: string } | null>(null)
  const [confirm, setConfirm] = useState(false)
  const form = draft ?? { provider: c?.provider ?? 'gemini', model: c?.model ?? DEFAULT_MODEL[c?.provider ?? 'gemini'], baseUrl: c?.baseUrl ?? '', apiKey: '' }
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) => setDraft({ ...form, [k]: v })
  const sameProvider = c?.configured && c.provider === form.provider
  const compat = form.provider === 'openai_compat'

  const submit = () => {
    const body: LLMConfigInput = { provider: form.provider, model: form.model.trim() }
    if (compat) body.baseUrl = form.baseUrl.trim()
    if (form.apiKey) body.apiKey = form.apiKey
    m.save.mutate(body, {
      onSuccess: () => {
        setDraft(null)
        toast({ title: 'IA enregistrée', description: 'Le bouton « Analyser avec mon IA » est disponible sur le tableau de bord.', tone: 'success' })
      },
    })
  }

  return (
    <Section id="ia" title="IA" icon={<Bot />} description="Ton propre fournisseur IA pour analyser une séance ou générer un plan. La clé est chiffrée côté serveur et n’est jamais réaffichée.">
      {cfg.isPending ? (
        <Spinner label="Chargement…" />
      ) : (
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          {c?.configured && (
            <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-ink-2">
              <CircleCheck className="size-4 text-ok" aria-hidden="true" />
              Connecté : <span className="font-semibold text-ink">{PROVIDER_LABEL[c.provider ?? ''] ?? c.provider}</span> · {c.model}
              {c.keyHint && <span className="num text-ink-3">· clé {c.keyHint}</span>}
              {c.updatedAt && <span className="text-xs text-ink-3">· mis à jour le {fmtDateTime(c.updatedAt)}</span>}
            </p>
          )}
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Fournisseur">
              {(p) => (
                <select
                  {...p}
                  className="field"
                  value={form.provider}
                  onChange={(e) => {
                    const provider = e.target.value as LLMProvider
                    setDraft({ ...form, provider, model: c?.provider === provider && c.model ? c.model : DEFAULT_MODEL[provider], apiKey: '' })
                  }}
                >
                  <option value="gemini">Gemini (Google)</option>
                  <option value="anthropic">Anthropic (Claude)</option>
                  <option value="openai">OpenAI</option>
                  <option value="openai_compat">Ollama / compatible OpenAI (URL)</option>
                </select>
              )}
            </Field>
            <Field label="Modèle">{(p) => <input {...p} className="field" value={form.model} onChange={(e) => set('model', e.target.value)} required autoComplete="off" spellCheck={false} />}</Field>
            {compat && (
              <Field label="URL de base" hint="Ex. http://ollama:11434 : l’appel part du serveur Stride (les adresses privées doivent être autorisées par l’administrateur).">
                {(p) => <input {...p} className="field" type="url" inputMode="url" placeholder="https://…" value={form.baseUrl} onChange={(e) => set('baseUrl', e.target.value)} required />}
              </Field>
            )}
            <Field label={compat ? 'Clé API (facultative)' : 'Clé API'} hint={sameProvider && c?.keyHint ? `Clé actuelle ${c.keyHint} : laisse vide pour la conserver.` : 'Écriture seule : elle ne sera plus affichée.'}>
              {(p) => <input {...p} className="field" type="password" autoComplete="new-password" spellCheck={false} placeholder={sameProvider && c?.keyHint ? `•••• ${c.keyHint}` : 'Colle ta clé'} value={form.apiKey} onChange={(e) => set('apiKey', e.target.value)} required={!compat && !(sameProvider && c?.keyHint)} />}
            </Field>
          </div>
          {m.save.isError && <InlineError>{m.save.error.message}</InlineError>}
          {m.test.data && (
            <p role="status" className={m.test.data.ok ? 'flex items-center gap-2 text-sm text-ok' : 'flex items-center gap-2 text-sm text-danger'}>
              {m.test.data.ok ? <CircleCheck className="size-4" aria-hidden="true" /> : <CircleX className="size-4" aria-hidden="true" />}
              {m.test.data.message} <span className="num text-ink-3">({m.test.data.latencyMs} ms)</span>
            </p>
          )}
          {m.test.isError && <InlineError>{m.test.error.message}</InlineError>}
          <div className="flex flex-wrap gap-2">
            <button type="submit" className="btn btn-primary" disabled={m.save.isPending}>
              {m.save.isPending && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
              Enregistrer
            </button>
            <button type="button" className="btn btn-ghost" disabled={!c?.configured || m.test.isPending || draft !== null} onClick={() => m.test.mutate()} title={draft ? 'Enregistre d’abord tes modifications' : undefined}>
              {m.test.isPending ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <PlugZap className="size-4" aria-hidden="true" />}
              Tester
            </button>
            {c?.configured && (
              <button type="button" className="btn btn-danger ml-auto" onClick={() => setConfirm(true)}>
                <Trash2 className="size-4" aria-hidden="true" /> Déconnecter
              </button>
            )}
          </div>
          <p className="text-xs text-ink-3">Le prompt envoyé est le même que « Copier le prompt » (directive d’objectivité en tête). Aucune donnée n’est envoyée sans ton action.</p>
        </form>
      )}
      <ConfirmDialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Déconnecter l’IA ?"
        message="La clé est supprimée du serveur. Les analyses déjà générées restent consultables."
        confirmLabel="Déconnecter"
        danger
        busy={m.remove.isPending}
        onConfirm={() => m.remove.mutate(undefined, { onSuccess: () => setConfirm(false) })}
      />
    </Section>
  )
}
