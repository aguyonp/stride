import { PageHeader } from '../../components/ui/Misc'
import { useDocumentTitle } from '../../hooks/useDocumentTitle'
import { ShoesSection } from '../settings/ShoesSection'
import { t } from '../../i18n'

export default function ShoesView() {
  useDocumentTitle(t('shoes.view.title'))
  return (
    <div className="space-y-5">
      <PageHeader
        title={t('shoes.view.title')}
        subtitle={t('shoes.view.subtitle')}
      />
      <ShoesSection />
    </div>
  )
}
