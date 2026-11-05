import type { ProviderStatus } from '../../api/types'
import { Badge } from '../ui/Badge'

export function ProviderBadge({ status, connected }: { status: ProviderStatus; connected: boolean }) {
  if (!connected || status === 'disconnected') return <span className="chip border border-line bg-track text-ink-2">Non connecté</span>
  if (status === 'error') return <Badge level="warn">Erreur</Badge>
  return <Badge level="ok">Connecté</Badge>
}
