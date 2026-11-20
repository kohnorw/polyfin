/*
 * This fork's additions to the users list (kohnorw/polyfin): the users can be selected, to turn
 * Live TV on or off for them all or delete them at once, and are listed a page at a time. Your
 * own account is never selected: it is changed on its own page.
 */
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { deleteUser, queryKeys, updateUser, type User } from '@/api'
import { useSessionUser } from '@/app/session'
import { useI18n } from '@/i18n'
import { Button, Checkbox, ConfirmDialog, RowList } from '@/ui'

const pageSize = 25

export function SelectableUsers({ users, row }: { users: User[]; row: (user: User) => ReactNode }) {
  const { t } = useI18n()
  const text = t.fork.users
  const session = useSessionUser()
  const [selected, setSelected] = useState<string[]>([])
  const [page, setPage] = useState(0)
  const pages = Math.ceil(users.length / pageSize)
  const current = Math.min(page, Math.max(pages - 1, 0))
  return (
    <>
      <BulkActions users={users} selected={selected} onSelect={setSelected} />
      <RowList aria-label={t.users.listTitle}>
        {users.slice(current * pageSize, (current + 1) * pageSize).map((user) => (
          <div key={user.id} className="flex items-center gap-1 ps-3 sm:gap-3">
            {user.id === session.id ? (
              <span aria-hidden="true" className="size-5 shrink-0" />
            ) : (
              <Checkbox
                label={user.name}
                hideLabel
                checked={selected.includes(user.id)}
                onChange={(on) =>
                  setSelected(on ? [...selected, user.id] : selected.filter((id) => id !== user.id))
                }
              />
            )}
            <div className="min-w-0 flex-1">{row(user)}</div>
          </div>
        ))}
      </RowList>
      {pages > 1 && (
        <div className="mt-3 flex items-center justify-between gap-3">
          <Button disabled={current === 0} onClick={() => setPage(current - 1)}>
            {text.previousPage}
          </Button>
          <span className="text-small text-ink-3 tabular-nums">
            {current + 1} / {pages}
          </span>
          <Button disabled={current >= pages - 1} onClick={() => setPage(current + 1)}>
            {text.nextPage}
          </Button>
        </div>
      )}
    </>
  )
}

function BulkActions({
  users,
  selected,
  onSelect,
}: {
  users: User[]
  selected: string[]
  onSelect: (ids: string[]) => void
}) {
  const { t } = useI18n()
  const text = t.fork.users
  const client = useQueryClient()
  const self = useSessionUser()
  const [confirming, setConfirming] = useState(false)
  const change = useMutation({
    mutationFn: (liveTv: boolean) =>
      Promise.allSettled(selected.map((id) => updateUser(id, { liveTv }))),
    onSettled: () => void client.invalidateQueries({ queryKey: queryKeys.users }),
  })
  const others = users.filter((u) => u.id !== self.id)
  const doomed = selected.filter((id) => id !== self.id)
  const removal = useMutation({
    mutationFn: () => Promise.allSettled(doomed.map((id) => deleteUser(id))),
    onSettled: () => {
      setConfirming(false)
      onSelect([])
      void client.invalidateQueries({ queryKey: queryKeys.users })
    },
  })
  return (
    <div className="mb-3 grid grid-cols-2 items-center gap-2 sm:flex sm:flex-wrap sm:gap-3">
      <div className="col-span-2 ps-3 sm:ps-0">
        <Checkbox
          label={text.selectAll}
          checked={others.length > 0 && selected.length === others.length}
          onChange={(all) => onSelect(all ? others.map((u) => u.id) : [])}
        />
      </div>
      <Button disabled={!selected.length || change.isPending} onClick={() => change.mutate(true)}>
        {text.liveTvOn}
      </Button>
      <Button disabled={!selected.length || change.isPending} onClick={() => change.mutate(false)}>
        {text.liveTvOff}
      </Button>
      <div className="col-span-2 grid sm:block">
        <Button disabled={!doomed.length || removal.isPending} onClick={() => setConfirming(true)}>
          {text.deleteSelected(doomed.length)}
        </Button>
      </div>
      <ConfirmDialog
        open={confirming}
        onClose={() => setConfirming(false)}
        onConfirm={() => removal.mutate()}
        title={text.deleteSelected(doomed.length)}
        confirmLabel={text.deleteSelected(doomed.length)}
      >
        {text.deleteSelectedHelp}
      </ConfirmDialog>
    </div>
  )
}
