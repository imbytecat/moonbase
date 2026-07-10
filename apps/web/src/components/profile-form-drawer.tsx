import { LeftOutlined } from '@ant-design/icons'
import { App, Button, Tag } from 'antd'
import { type ReactNode, useState } from 'react'
import { FormDrawer } from '#components/form-drawer'

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
  dirty,
  profileProvider,
  providers,
  children,
}: {
  open: boolean
  onClose: () => void
  dirty: boolean
  profileProvider: string | undefined
  providers: ProviderOption[]
  children: (provider: string) => ReactNode
}) {
  const { modal } = App.useApp()
  const isNew = profileProvider === undefined
  const [picked, setPicked] = useState(profileProvider)
  const active = providers.find((p) => p.value === picked)

  const backToPicker = () => {
    const discard = () => setPicked(undefined)
    if (!dirty) {
      discard()
      return
    }
    modal.confirm({
      title: '放弃未保存的更改？',
      content: '关闭后已填写的内容将丢失',
      okText: '放弃更改',
      okButtonProps: { danger: true },
      cancelText: '继续编辑',
      onOk: discard,
    })
  }

  return (
    <FormDrawer
      title={isNew ? '添加配置' : '编辑配置'}
      open={open}
      onClose={onClose}
      dirty={active ? dirty : false}
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
                {'重新选择'}
              </Button>
            ) : (
              <Tag className="!me-0">{'创建后不可更改'}</Tag>
            )}
          </div>
          {children(active.value)}
        </>
      ) : (
        <div className="space-y-3">
          <div className="text-sm text-(--ant-color-text-secondary)">
            {'选择服务类型，创建后不可更改'}
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
