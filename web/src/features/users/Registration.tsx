import { ArrowSquareOutIcon } from '@phosphor-icons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchSettings, queryKeys, saveSettings } from '@/api'
import { useI18n } from '@/i18n'
import { Block, ExternalButtonLink, Switch } from '@/ui'

/**
 * The Users page's switch for /register, where visitors create their own
 * member account (Settings.registration), with a link to the page.
 */
export default function Registration() {
  const { t } = useI18n()
  const s = t.users
  const client = useQueryClient()
  const settings = useQuery({
    queryKey: queryKeys.settings,
    queryFn: ({ signal }) => fetchSettings(signal),
  })
  const save = useMutation({
    mutationFn: (registration: boolean) => saveSettings({ ...settings.data!, registration }),
    onSuccess: (saved) => client.setQueryData(queryKeys.settings, saved),
  })
  const open = save.isPending ? save.variables : (settings.data?.registration ?? false)
  return (
    <Block title={s.registrationTitle}>
      <div className="flex flex-wrap items-center justify-between gap-4 rounded-row border border-line bg-s1 px-5 py-4">
        <div className="min-w-0 max-w-[60ch]">
          <p className="text-[15px] font-medium text-ink">{s.registration}</p>
          <p className="mt-1 text-small text-ink-3">{s.registrationHelp}</p>
          {save.isError && <p className="mt-1 text-small text-danger">{s.registrationFailed}</p>}
        </div>
        <div className="flex items-center gap-3">
          <ExternalButtonLink
            href="/register"
            target="_blank"
            rel="noopener"
            iconEnd={ArrowSquareOutIcon}
          >
            {s.registrationOpenPage}
          </ExternalButtonLink>
          <Switch
            checked={open}
            disabled={!settings.data || save.isPending}
            onChange={(checked) => save.mutate(checked)}
            label={s.registration}
          />
        </div>
      </div>
    </Block>
  )
}
