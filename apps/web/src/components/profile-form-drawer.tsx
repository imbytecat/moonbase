import { LeftOutlined } from '@ant-design/icons'
import type { ProviderDescriptor } from '@moonbase/api-client'
import { App, Button, Tag } from 'antd'
import { type ReactNode, useState } from 'react'
import { FormDrawer } from '#components/form-drawer'
import { ProviderIcon } from '#components/provider-icon'

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
  providers: ProviderDescriptor[]
  children: (provider: string) => ReactNode
}) {
  const { modal } = App.useApp()
  const isNew = profileProvider === undefined
  const [picked, setPicked] = useState(profileProvider)
  const active = providers.find((provider) => provider.key === picked)

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
              <ProviderIcon
                iconRef={active.presentation?.iconRef ?? ''}
                color={active.presentation?.color}
              />
              <span className="truncate font-medium">
                {active.presentation?.name || active.key}
              </span>
            </span>
            {isNew ? (
              <Button type="text" size="small" icon={<LeftOutlined />} onClick={backToPicker}>
                {'重新选择'}
              </Button>
            ) : (
              <Tag className="!me-0">{'创建后不可更改'}</Tag>
            )}
          </div>
          {children(active.key)}
        </>
      ) : (
        <div className="space-y-3">
          <div className="text-sm text-(--ant-color-text-secondary)">
            {'选择服务类型，创建后不可更改'}
          </div>
          {providers.map((provider) => (
            <button
              key={provider.key}
              type="button"
              onClick={() => setPicked(provider.key)}
              className="flex w-full cursor-pointer items-center gap-3 rounded-lg border border-solid border-(--ant-color-border) bg-transparent p-4 text-start transition-colors hover:border-(--ant-color-primary)"
            >
              <ProviderIcon
                iconRef={provider.presentation?.iconRef ?? ''}
                color={provider.presentation?.color}
                className="text-xl"
              />
              <span className="min-w-0">
                <span className="block font-medium text-(--ant-color-text)">
                  {provider.presentation?.name || provider.key}
                </span>
                <span className="block text-xs text-(--ant-color-text-tertiary)">
                  {provider.presentation?.description}
                </span>
              </span>
            </button>
          ))}
        </div>
      )}
    </FormDrawer>
  )
}
