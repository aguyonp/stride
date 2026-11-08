import type { ProviderStatus } from '../../api/types'
import { Badge } from '../ui/Badge'
import { t } from '../../i18n'

export function ProviderBadge({ status, connected }: { status: ProviderStatus; connected: boolean }) {
  if (!connected || status === 'disconnected') return <span className="chip border border-line bg-track text-ink-2">{t('settings.providers.notConnected')}</span>
  if (status === 'error') return <Badge level="warn">{t('settings.providers.error')}</Badge>
  return <Badge level="ok">{t('settings.providers.connected')}</Badge>
}
