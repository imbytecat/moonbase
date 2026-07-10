import { useCallback, useRef, useState } from 'react'

// Drives an add/edit drawer's mount lifecycle so its close animation survives.
//
// The drawer's React key (drawerKey) advances only when a *new* target opens —
// never on close. antd therefore keeps the same drawer instance while it
// animates shut (leave transition intact), then a fresh instance mounts on the
// next open with clean form + provider-picker state. Deriving the key from the
// target instead (the previous `editing?.id ?? 'closed'` approach) folded a
// "closed" sentinel into it, remounting the open drawer mid-close and
// preempting the leave animation.
export type EditingTarget<T> = T | 'new'

export interface EditingTargetControls<T> {
  // The profile being edited, or undefined when creating or closed. Feed this to
  // the drawer's `profile` prop directly.
  profile: T | undefined
  // Whether the drawer is open. True while creating or editing.
  open: boolean
  // Stable React key for the drawer; advances on open, never on close.
  drawerKey: string
  add: () => void
  edit: (profile: T) => void
  close: () => void
}

export function useEditingTarget<T>(): EditingTargetControls<T> {
  const [target, setTarget] = useState<EditingTarget<T> | undefined>()
  const [drawerKey, setDrawerKey] = useState('closed')
  const seq = useRef(0)

  const openTarget = useCallback((next: EditingTarget<T>) => {
    seq.current += 1
    setDrawerKey(`k${seq.current}`)
    setTarget(next)
  }, [])

  const add = useCallback(() => openTarget('new'), [openTarget])
  const edit = useCallback((profile: T) => openTarget(profile), [openTarget])
  const close = useCallback(() => setTarget(undefined), [])

  return {
    profile: target === 'new' ? undefined : target,
    open: target !== undefined,
    drawerKey,
    add,
    edit,
    close,
  }
}
