import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import { Button, Empty, Popconfirm, Select, Tag, Typography } from 'antd'
import type { ReactNode } from 'react'

interface ProfileLike {
  id: string
  name: string
}

interface BindingRow {
  purpose: string
  profileIds: string[]
  // Multi-valued purposes (e.g. third-party login) render a multi-select;
  // single-valued ones a clearable single select.
  multiple?: boolean
}

export function ProviderTag({ name }: { name: string }) {
  return <Tag className="!me-0">{name}</Tag>
}

interface ProfileManagerTexts {
  profilesTitle: string
  profilesHint: string
  noProfiles: string
  confirmDelete: string
  bindingsHint: string
}

interface ProfileManagerProps<T extends ProfileLike> {
  profiles: T[]
  bindings: BindingRow[]
  texts: ProfileManagerTexts
  purposeLabel: (purpose: string) => string
  profileIcon: (profile: T) => ReactNode
  profileTags?: (profile: T) => ReactNode
  profileDescription: (profile: T) => ReactNode
  onAdd: () => void
  onEdit: (profile: T) => void
  onDelete: (profile: T) => void
  deleting: boolean
  // Prevents delete with a tooltip-worthy reason when set.
  deleteDisabled?: (profile: T) => boolean
  onBind: (purpose: string, profileIds: string[]) => void
  binding: boolean
}

export function ProfileManager<T extends ProfileLike>({
  profiles,
  bindings,
  texts,
  purposeLabel,
  profileIcon,
  profileTags,
  profileDescription,
  onAdd,
  onEdit,
  onDelete,
  deleting,
  deleteDisabled,
  onBind,
  binding,
}: ProfileManagerProps<T>) {
  const boundPurposes = (profileId: string) =>
    bindings.filter((b) => b.profileIds.includes(profileId)).map((b) => b.purpose)

  return (
    <div className="space-y-6">
      <div>
        <div className="mb-3 flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
          <div className="min-w-40 flex-1">
            <Typography.Text strong>{texts.profilesTitle}</Typography.Text>
            <div className="text-xs text-(--ant-color-text-tertiary)">{texts.profilesHint}</div>
          </div>
          <Button type="primary" icon={<PlusOutlined />} onClick={onAdd} className="shrink-0">
            {'添加配置'}
          </Button>
        </div>

        {profiles.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={texts.noProfiles} />
        ) : (
          <ul className="m-0 list-none divide-y divide-(--ant-color-split) rounded-lg border border-(--ant-color-border) p-0">
            {profiles.map((p) => {
              const bound = boundPurposes(p.id)
              const undeletable = bound.length > 0 || (deleteDisabled?.(p) ?? false)
              return (
                <li key={p.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                  <div className="flex min-w-0 flex-1 basis-52 items-start gap-3">
                    <span className="shrink-0 pt-0.5">{profileIcon(p)}</span>
                    <div className="min-w-0">
                      <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="whitespace-nowrap font-medium">{p.name}</span>
                        {profileTags?.(p)}
                        {bound.map((purpose) => (
                          <Tag key={purpose} color="blue" className="!me-0">
                            {purposeLabel(purpose)}
                          </Tag>
                        ))}
                        {bound.length === 0 ? (
                          <Tag className="!me-0 !text-(--ant-color-text-quaternary)">
                            {'未使用'}
                          </Tag>
                        ) : null}
                      </span>
                      <div className="text-sm text-(--ant-color-text-tertiary)">
                        {profileDescription(p)}
                      </div>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button type="text" icon={<EditOutlined />} onClick={() => onEdit(p)} />
                    <Popconfirm
                      title={texts.confirmDelete}
                      onConfirm={() => onDelete(p)}
                      disabled={undeletable}
                    >
                      <Button
                        type="text"
                        danger
                        icon={<DeleteOutlined />}
                        disabled={undeletable}
                        loading={deleting}
                      />
                    </Popconfirm>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      <div>
        <div className="mb-3">
          <Typography.Text strong>{'用途绑定'}</Typography.Text>
          <div className="text-xs text-(--ant-color-text-tertiary)">{texts.bindingsHint}</div>
        </div>
        <div className="space-y-3">
          {bindings.map((b) => (
            <div
              key={b.purpose}
              className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1"
            >
              <span className="text-sm">{purposeLabel(b.purpose)}</span>
              {b.multiple ? (
                <Select
                  className="w-full min-w-0 sm:w-64"
                  mode="multiple"
                  value={b.profileIds}
                  placeholder={'未绑定'}
                  loading={binding}
                  options={profiles.map((p) => ({ label: p.name, value: p.id }))}
                  onChange={(ids) => onBind(b.purpose, ids)}
                />
              ) : (
                <Select
                  className="w-full min-w-0 sm:w-64"
                  value={b.profileIds[0]}
                  placeholder={'未绑定'}
                  allowClear
                  loading={binding}
                  options={profiles.map((p) => ({ label: p.name, value: p.id }))}
                  onChange={(id) => onBind(b.purpose, id ? [id] : [])}
                />
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
