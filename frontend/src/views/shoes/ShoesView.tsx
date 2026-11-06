import { PageHeader } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { ShoesSection } from '../settings/ShoesSection'

export default function ShoesView() {
  useDocumentTitle('Chaussures')
  return (
    <div className="space-y-5">
      <PageHeader
        title="Chaussures"
        subtitle="Rotation active, drop, type de semelle et kilométrage cumulé par paire."
      />
      <ShoesSection />
    </div>
  )
}
