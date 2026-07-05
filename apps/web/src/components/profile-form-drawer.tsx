import { LeftOutlined } from '@ant-design/icons'
import { App, Button, type FormInstance, Tag } from 'antd'
import { type ReactNode, useState } from 'react'
import { FormDrawer } from '#components/form-drawer'
import { m } from '#paraglide/messages.js'

export interface ProviderOption {
  value: string
  label: string
  description: string
  icon: ReactNode
}

// Two-step profile drawer (the Logto connector shape): creating first picks a
// provider from descriptive cards, then fills only that provider's form.
// Editing skips the picker — the provider is fixed at creation; switching
// means creating a new profile and rebinding, so half-configured hybrids
// can't exist.
export function ProfileFormDrawer({
  open,
  onClose,
  form,
  profileProvider,
  providers,
  children,
}: {
  open: boolean
  onClose: () => void
  form: FormInstance
  profileProvider: string | undefined
  providers: ProviderOption[]
  children: (provider: string) => ReactNode
}) {
  const { modal } = App.useApp()
  const isNew = profileProvider === undefined
  const [picked, setPicked] = useState(profileProvider)
  const active = providers.find((p) => p.value === picked)

  const backToPicker = () => {
    const discard = () => {
      form.resetFields()
      setPicked(undefined)
    }
    if (!form.isFieldsTouched()) {
      discard()
      return
    }
    modal.confirm({
      title: m.common_unsavedTitle(),
      content: m.common_unsavedContent(),
      okText: m.common_discard(),
      okButtonProps: { danger: true },
      cancelText: m.common_keepEditing(),
      onOk: discard,
    })
  }

  return (
    <FormDrawer
      title={isNew ? m.systemPage_addProfile() : m.systemPage_editProfile()}
      open={open}
      onClose={onClose}
      form={active ? form : undefined}
    >
      {active ? (
        <>
          <div className="mb-4 flex items-center justify-between gap-2 rounded-lg bg-(--ant-color-fill-quaternary) px-3 py-2">
            <span className="flex min-w-0 items-center gap-2">
              {active.icon}
              <span className="truncate font-medium">{active.label}</span>
            </span>
            {isNew ? (
              <Button type="text" size="small" icon={<LeftOutlined />} onClick={backToPicker}>
                {m.systemPage_changeProvider()}
              </Button>
            ) : (
              <Tag className="!me-0">{m.systemPage_providerLocked()}</Tag>
            )}
          </div>
          {children(active.value)}
        </>
      ) : (
        <div className="space-y-3">
          <div className="text-sm text-(--ant-color-text-secondary)">
            {m.systemPage_pickProviderHint()}
          </div>
          {providers.map((p) => (
            <button
              key={p.value}
              type="button"
              onClick={() => setPicked(p.value)}
              className="flex w-full cursor-pointer items-center gap-3 rounded-lg border border-solid border-(--ant-color-border) bg-transparent p-4 text-start transition-colors hover:border-(--ant-color-primary)"
            >
              {p.icon}
              <span className="min-w-0">
                <span className="block font-medium text-(--ant-color-text)">{p.label}</span>
                <span className="block text-xs text-(--ant-color-text-tertiary)">
                  {p.description}
                </span>
              </span>
            </button>
          ))}
        </div>
      )}
    </FormDrawer>
  )
}
