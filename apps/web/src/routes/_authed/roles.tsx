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
      message.success('角色已创建')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const updateMutation = useMutation(updateRole, {
    onSuccess: () => {
      void invalidate()
      setDrawer({ open: false })
      message.success('角色已更新')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const deleteMutation = useMutation(deleteRole, {
    onSuccess: () => {
      void invalidate()
      message.success('角色已删除')
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
      title={'角色管理'}
      extra={
        canWrite ? (
          <Button type="primary" onClick={openCreate}>
            {'添加角色'}
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
            title: '角色',
            key: 'role',
            render: (_, r) => (
              <div>
                <span className="font-medium">{r.name}</span>
                {r.isSystem ? (
                  <Tag className="ms-2" color="blue">
                    {'系统'}
                  </Tag>
                ) : null}
                <div className="text-xs text-gray-500">{r.description}</div>
              </div>
            ),
          },
          {
            title: '权限',
            dataIndex: 'permissions',
            render: (perms: Permission[]) =>
              perms.includes(Permission.ALL) ? (
                <Tag color="gold">{'全部权限'}</Tag>
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
                        {'编辑'}
                      </Button>
                      {r.isSystem ? null : (
                        <Popconfirm
                          title={'删除该角色？'}
                          okText={'删除'}
                          okButtonProps={{ danger: true }}
                          onConfirm={() => deleteMutation.mutate({ id: r.id })}
                        >
                          <Button size="small" danger>
                            {'删除'}
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
        title={drawer.editing ? `编辑 ${drawer.editing.name}` : '添加角色'}
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
            label={'角色名称'}
            rules={[{ required: true, message: '请输入角色名' }]}
          >
            <Input disabled={drawer.editing?.isSystem} />
          </Form.Item>
          <Form.Item name="description" label={'描述'}>
            <Input.TextArea rows={2} />
          </Form.Item>
          {drawer.editing?.isSystem && drawer.editing.name === 'admin' ? (
            <Form.Item label={'权限'}>
              <Tag color="gold">{'全部权限（不可修改）'}</Tag>
            </Form.Item>
          ) : (
            <Form.Item name="permissions" label={'权限'}>
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
            {drawer.editing ? '保存修改' : '创建角色'}
          </Button>
        </Form>
      </Drawer>
    </Card>
  )
}
