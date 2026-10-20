import { ArrowSquareOutIcon, CopyIcon, PlusIcon, TrashIcon } from '@phosphor-icons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import {
  createRegistrationCodes,
  deleteRegistrationCode,
  fetchRegistrationCodes,
  fetchSettings,
  queryKeys,
  saveSettings,
  type Settings,
} from '@/api'
import { useI18n } from '@/i18n'
import { Block, Button, ExternalButtonLink, IconButton, Switch } from '@/ui'

/**
 * The Users page's switches for /register, where visitors create their own
 * member account (Settings.registration), and for the registration codes
 * they must type first (Settings.registrationCodes), with the codes.
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
    mutationFn: (change: Partial<Settings>) => saveSettings({ ...settings.data!, ...change }),
    onSuccess: (saved) => client.setQueryData(queryKeys.settings, saved),
  })
  const pending = save.isPending ? save.variables : {}
  const open = pending.registration ?? settings.data?.registration ?? false
  const codesOn = pending.registrationCodes ?? settings.data?.registrationCodes ?? false
  return (
    <Block title={s.registrationTitle}>
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-4 rounded-row border border-line bg-s1 px-5 py-4">
          <div className="min-w-0 max-w-[60ch]">
            <p className="text-[15px] font-medium text-ink">{s.registration}</p>
            <p className="mt-1 text-small text-ink-3">{s.registrationHelp}</p>
            {save.isError && <p className="mt-1 text-small text-danger">{s.registrationFailed}</p>}
          </div>
          <div className="flex items-center gap-3">
            <ExternalButtonLink href="/register" target="_blank" rel="noopener" iconEnd={ArrowSquareOutIcon}>
              {s.registrationOpenPage}
            </ExternalButtonLink>
            <Switch
              checked={open}
              disabled={!settings.data || save.isPending}
              onChange={(registration) => save.mutate({ registration })}
              label={s.registration}
            />
          </div>
        </div>
        <div className="rounded-row border border-line bg-s1 px-5 py-4">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="min-w-0 max-w-[60ch]">
              <p className="text-[15px] font-medium text-ink">{s.registrationCodes}</p>
              <p className="mt-1 text-small text-ink-3">{s.registrationCodesHelp}</p>
            </div>
            <Switch
              checked={codesOn}
              disabled={!settings.data || save.isPending}
              onChange={(registrationCodes) => save.mutate({ registrationCodes })}
              label={s.registrationCodes}
            />
          </div>
          {codesOn && <Codes />}
        </div>
      </div>
    </Block>
  )
}

function Codes() {
  const { t } = useI18n()
  const s = t.users
  const client = useQueryClient()
  const [copied, setCopied] = useState('')
  const codes = useQuery({
    queryKey: queryKeys.registrationCodes,
    queryFn: ({ signal }) => fetchRegistrationCodes(signal),
  })
  const refresh = () => client.invalidateQueries({ queryKey: queryKeys.registrationCodes })
  const create = useMutation({ mutationFn: () => createRegistrationCodes(1), onSuccess: refresh })
  const remove = useMutation({ mutationFn: deleteRegistrationCode, onSuccess: refresh })
  const copy = (code: string) => {
    void navigator.clipboard?.writeText(code).then(() => {
      setCopied(code)
      setTimeout(() => setCopied(''), 1500)
    })
  }
  return (
    <div className="mt-4 border-t border-line pt-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-small text-ink-3">
          {codes.data ? s.registrationCodesCount(codes.data.filter((c) => !c.usedAt).length) : ''}
        </p>
        <Button size="sm" icon={PlusIcon} loading={create.isPending} onClick={() => create.mutate()}>
          {s.registrationCodesNew}
        </Button>
      </div>
      {(create.isError || remove.isError) && (
        <p className="mt-2 text-small text-danger">{s.registrationCodesFailed}</p>
      )}
      {codes.data && codes.data.length > 0 && (
        <ul className="mt-3 divide-y divide-line">
          {codes.data.map((code) => (
            <li key={code.code} className="flex items-center gap-3 py-2">
              <span className={code.usedAt ? 'font-mono text-ink-3 line-through' : 'font-mono text-ink'}>
                {code.code}
              </span>
              <span className="min-w-0 flex-1 truncate text-small text-ink-3">
                {code.usedAt ? s.registrationCodeUsed(code.usedBy ?? '—') : s.registrationCodeUnused}
              </span>
              {!code.usedAt && (
                <IconButton
                  size="sm"
                  icon={CopyIcon}
                  label={copied === code.code ? s.registrationCodeCopied : s.registrationCodeCopy}
                  onClick={() => copy(code.code)}
                />
              )}
              <IconButton
                size="sm"
                icon={TrashIcon}
                danger
                label={s.registrationCodeDelete}
                onClick={() => remove.mutate(code.code)}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
