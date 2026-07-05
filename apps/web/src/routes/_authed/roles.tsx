import {
  createConnectQueryKey,
  createQueryOptions,
  useMutation,
  useQuery,
  useSuspenseQuery,
} from '@connectrpc/connect-query'
import {
  createRole,
  deleteRole,
  getMe,
  listPermissions,
  listRoles,
  Permission,
  type Role,
  updateRole,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { App, Button, Card, Drawer, Form, Input, Popconfirm, Table, Tag } from 'antd'
import { useState } from 'react'
import { PermissionChecklist } from '#components/permission-checklist'
import { humanizeError } from '#lib/errors'
import { permissionKey } from '#lib/permissions'
import { hasPermission, requirePermission } from '#lib/session'
import { m } from '#paraglide/messages.js'

export const Route = createFileRoute('/_authed/roles')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.ROLE_READ),
  loader: ({ context: { queryClient, transport } }) =>
    Promise.all([
      queryClient.ensureQueryData(createQueryOptions(listRoles, undefined, { transport })),
      queryClient.ensureQueryData(createQueryOptions(listPermissions, undefined, { transport })),
    ]),
  component: RolesPage,
})

interface RoleFormValues {
  name: string
  description: string
  permissions: Permission[]
}

function RolesPage() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { data } = useSuspenseQuery(listRoles)
  const { data: permsData } = useSuspenseQuery(listPermissions)
  const { data: meData } = useQuery(getMe)
  const canWrite = hasPermission(meData?.user, Permission.ROLE_WRITE)

  const [drawer, setDrawer] = useState<{ open: boolean; editing?: Role }>({ open: false })
  const [form] = Form.useForm<RoleFormValues>()

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listRoles, cardinality: 'finite' }),
    })

  const createMutation = useMutation(createRole, {
    onSuccess: () => {
      void invalidate()
      setDrawer({ open: false })
      message.success(m.rolesPage_roleCreated())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const updateMutation = useMutation(updateRole, {
    onSuccess: () => {
      void invalidate()
      setDrawer({ open: false })
      message.success(m.rolesPage_roleUpdated())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const deleteMutation = useMutation(deleteRole, {
    onSuccess: () => {
      void invalidate()
      message.success(m.rolesPage_roleDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ permissions: [] })
    setDrawer({ open: true })
  }

  const openEdit = (role: Role) => {
    form.setFieldsValue({
      name: role.name,
      description: role.description,
      permissions: role.permissions,
    })
    setDrawer({ open: true, editing: role })
  }

  const submit = (values: RoleFormValues) => {
    if (drawer.editing) {
      updateMutation.mutate({ id: drawer.editing.id, ...values })
    } else {
      createMutation.mutate(values)
    }
  }

  return (
    <Card
      title={m.rolesPage_title()}
      extra={
        canWrite ? (
          <Button type="primary" onClick={openCreate}>
            {m.rolesPage_addRole()}
          </Button>
        ) : null
      }
    >
      <Table<Role>
        rowKey="id"
        dataSource={data.roles}
        pagination={false}
        scroll={{ x: 'max-content' }}
        columns={[
          {
            title: m.rolesPage_role(),
            key: 'role',
            render: (_, r) => (
              <div>
                <span className="font-medium">{r.name}</span>
                {r.isSystem ? (
                  <Tag className="ms-2" color="blue">
                    {m.rolesPage_system()}
                  </Tag>
                ) : null}
                <div className="text-xs text-gray-500">{r.description}</div>
              </div>
            ),
          },
          {
            title: m.rolesPage_permissions(),
            dataIndex: 'permissions',
            render: (perms: Permission[]) =>
              perms.includes(Permission.ALL) ? (
                <Tag color="gold">{m.rolesPage_allPermissions()}</Tag>
              ) : (
                perms.map((p) => <Tag key={p}>{permissionKey(p)}</Tag>)
              ),
          },
          ...(canWrite
            ? [
                {
                  title: '',
                  key: 'actions',
                  width: 140,
                  render: (_: unknown, r: Role) => (
                    <div className="flex gap-2">
                      <Button size="small" onClick={() => openEdit(r)}>
                        {m.common_edit()}
                      </Button>
                      {r.isSystem ? null : (
                        <Popconfirm
                          title={m.rolesPage_confirmDelete()}
                          okText={m.common_delete()}
                          okButtonProps={{ danger: true }}
                          onConfirm={() => deleteMutation.mutate({ id: r.id })}
                        >
                          <Button size="small" danger>
                            {m.common_delete()}
                          </Button>
                        </Popconfirm>
                      )}
                    </div>
                  ),
                },
              ]
            : []),
        ]}
      />

      <Drawer
        title={
          drawer.editing
            ? m.rolesPage_editRole({ name: drawer.editing.name })
            : m.rolesPage_addRole()
        }
        open={drawer.open}
        onClose={() => setDrawer({ open: false })}
        size="min(420px, 100vw)"
        destroyOnHidden
      >
        <Form<RoleFormValues>
          form={form}
          layout="vertical"
          requiredMark={false}
          onFinish={submit}
          disabled={createMutation.isPending || updateMutation.isPending}
        >
          <Form.Item
            name="name"
            label={m.rolesPage_name()}
            rules={[{ required: true, message: m.rolesPage_nameRequired() }]}
          >
            <Input disabled={drawer.editing?.isSystem} />
          </Form.Item>
          <Form.Item name="description" label={m.rolesPage_description()}>
            <Input.TextArea rows={2} />
          </Form.Item>
          {drawer.editing?.isSystem && drawer.editing.name === 'admin' ? (
            <Form.Item label={m.rolesPage_permissions()}>
              <Tag color="gold">{m.rolesPage_allPermissionsImmutable()}</Tag>
            </Form.Item>
          ) : (
            <Form.Item name="permissions" label={m.rolesPage_permissions()}>
              <PermissionChecklist
                options={permsData.permissions.map((p) => ({
                  permission: p.permission,
                  description: p.description,
                }))}
              />
            </Form.Item>
          )}
          <Button
            type="primary"
            htmlType="submit"
            block
            loading={createMutation.isPending || updateMutation.isPending}
          >
            {drawer.editing ? m.rolesPage_saveChanges() : m.rolesPage_createRole()}
          </Button>
        </Form>
      </Drawer>
    </Card>
  )
}
