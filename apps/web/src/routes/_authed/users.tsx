import { timestampDate } from '@bufbuild/protobuf/wkt'
import {
  createConnectQueryKey,
  createQueryOptions,
  useMutation,
  useQuery,
  useSuspenseQuery,
} from '@connectrpc/connect-query'
import {
  createUser,
  deleteUser,
  getMe,
  listRoles,
  listUsers,
  Permission,
  resetUserPassword,
  type User,
  updateUser,
} from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  App,
  Avatar,
  Button,
  Card,
  Drawer,
  Form,
  Input,
  Popconfirm,
  Select,
  Switch,
  Table,
  Tag,
} from 'antd'
import { useState } from 'react'
import { humanizeError } from '#lib/errors'
import { hasPermission, requirePermission } from '#lib/session'
import { m } from '#paraglide/messages.js'

export const Route = createFileRoute('/_authed/users')({
  beforeLoad: ({ context: { queryClient, transport } }) =>
    requirePermission(queryClient, transport, Permission.USER_READ),
  loader: ({ context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(createQueryOptions(listUsers, undefined, { transport })),
  component: UsersPage,
})

interface UserFormValues {
  username?: string
  email?: string
  name: string
  password?: string
  roleIds: string[]
  isActive: boolean
}

function UsersPage() {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const { data } = useSuspenseQuery(listUsers)
  const { data: rolesData } = useQuery(listRoles)
  const { data: meData } = useQuery(getMe)
  const canWrite = hasPermission(meData?.user, Permission.USER_WRITE)

  const [drawer, setDrawer] = useState<{ open: boolean; editing?: User }>({ open: false })
  const [form] = Form.useForm<UserFormValues>()

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listUsers, cardinality: 'finite' }),
    })

  const createMutation = useMutation(createUser, {
    onSuccess: () => {
      void invalidate()
      setDrawer({ open: false })
      message.success(m.usersPage_userCreated())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const updateMutation = useMutation(updateUser, {
    onSuccess: () => {
      void invalidate()
      setDrawer({ open: false })
      message.success(m.usersPage_userUpdated())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const deleteMutation = useMutation(deleteUser, {
    onSuccess: () => {
      void invalidate()
      message.success(m.usersPage_userDeleted())
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const resetPasswordMutation = useMutation(resetUserPassword, {
    onSuccess: () => message.success(m.usersPage_passwordReset()),
    onError: (err) => message.error(humanizeError(err)),
  })

  const [resetTarget, setResetTarget] = useState<User>()
  const [resetForm] = Form.useForm<{ newPassword: string }>()

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ isActive: true, roleIds: [] })
    setDrawer({ open: true })
  }

  const openEdit = (user: User) => {
    form.setFieldsValue({
      username: user.username,
      email: user.email,
      name: user.name,
      roleIds: user.roleIds,
      isActive: user.isActive,
    })
    setDrawer({ open: true, editing: user })
  }

  const submit = (values: UserFormValues) => {
    if (drawer.editing) {
      updateMutation.mutate({
        id: drawer.editing.id,
        email: values.email || undefined,
        name: values.name,
        isActive: values.isActive,
        roles: { roleIds: values.roleIds },
      })
    } else {
      createMutation.mutate({
        username: values.username ?? '',
        email: values.email ?? '',
        name: values.name,
        password: values.password ?? '',
        roleIds: values.roleIds,
      })
    }
  }

  const roleOptions = (rolesData?.roles ?? []).map((r) => ({ label: r.name, value: r.id }))

  return (
    <Card
      title={m.usersPage_title()}
      extra={
        canWrite ? (
          <Button type="primary" onClick={openCreate}>
            {m.usersPage_addUser()}
          </Button>
        ) : null
      }
    >
      <Table<User>
        rowKey="id"
        dataSource={data.users}
        pagination={{ pageSize: 20, hideOnSinglePage: true }}
        scroll={{ x: 'max-content' }}
        columns={[
          {
            title: m.usersPage_user(),
            key: 'user',
            render: (_, u) => (
              <div className="flex items-center gap-3">
                <Avatar src={u.avatarUrl || undefined}>{u.name.charAt(0).toUpperCase()}</Avatar>
                <div>
                  <div>{u.name}</div>
                  <div className="text-xs text-gray-500">
                    {[u.username, u.email].filter(Boolean).join(' · ')}
                  </div>
                </div>
              </div>
            ),
          },
          {
            title: m.usersPage_rolesCol(),
            dataIndex: 'roles',
            render: (roles: string[]) =>
              roles.length ? (
                roles.map((r) => <Tag key={r}>{r}</Tag>)
              ) : (
                <Tag>{m.usersPage_noRoles()}</Tag>
              ),
          },
          {
            title: m.usersPage_status(),
            dataIndex: 'isActive',
            render: (active: boolean) =>
              active ? (
                <Tag color="green">{m.usersPage_active()}</Tag>
              ) : (
                <Tag color="red">{m.usersPage_disabled()}</Tag>
              ),
          },
          {
            title: m.usersPage_created(),
            dataIndex: 'createdAt',
            render: (_, u) => (u.createdAt ? timestampDate(u.createdAt).toLocaleDateString() : ''),
          },
          ...(canWrite
            ? [
                {
                  title: '',
                  key: 'actions',
                  width: 220,
                  render: (_: unknown, u: User) => (
                    <div className="flex gap-2">
                      <Button size="small" onClick={() => openEdit(u)}>
                        {m.common_edit()}
                      </Button>
                      <Button
                        size="small"
                        onClick={() => {
                          resetForm.resetFields()
                          setResetTarget(u)
                        }}
                      >
                        {m.usersPage_resetPassword()}
                      </Button>
                      <Popconfirm
                        title={m.usersPage_confirmDelete()}
                        okText={m.common_delete()}
                        okButtonProps={{ danger: true }}
                        onConfirm={() => deleteMutation.mutate({ id: u.id })}
                      >
                        <Button size="small" danger>
                          {m.common_delete()}
                        </Button>
                      </Popconfirm>
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
            ? m.usersPage_editUser({ name: drawer.editing.name })
            : m.usersPage_addUser()
        }
        open={drawer.open}
        onClose={() => setDrawer({ open: false })}
        size="min(420px, 100vw)"
        destroyOnHidden
      >
        <Form<UserFormValues>
          form={form}
          layout="vertical"
          requiredMark={false}
          onFinish={submit}
          disabled={createMutation.isPending || updateMutation.isPending}
        >
          <Form.Item
            name="name"
            label={m.auth_name()}
            rules={[{ required: true, message: m.auth_nameRule() }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="username"
            label={m.auth_username()}
            dependencies={['email']}
            rules={[
              { pattern: /^[a-zA-Z][a-zA-Z0-9._-]{2,31}$/, message: m.auth_usernameRule() },
              ({ getFieldValue }) => ({
                validator: () =>
                  getFieldValue('username') || getFieldValue('email')
                    ? Promise.resolve()
                    : Promise.reject(new Error(m.usersPage_identifierRequired())),
              }),
            ]}
          >
            <Input disabled={Boolean(drawer.editing)} />
          </Form.Item>
          <Form.Item
            name="email"
            label={m.auth_email()}
            dependencies={['username']}
            rules={[
              { type: 'email', message: m.auth_emailRule() },
              ({ getFieldValue }) => ({
                validator: () =>
                  getFieldValue('username') || getFieldValue('email')
                    ? Promise.resolve()
                    : Promise.reject(new Error(m.usersPage_identifierRequired())),
              }),
            ]}
          >
            <Input />
          </Form.Item>
          {drawer.editing ? null : (
            <Form.Item
              name="password"
              label={m.auth_password()}
              rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
          )}
          <Form.Item name="roleIds" label={m.usersPage_rolesCol()}>
            <Select mode="multiple" options={roleOptions} placeholder={m.usersPage_assignRoles()} />
          </Form.Item>
          <Form.Item name="isActive" label={m.usersPage_activeLabel()} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Button
            type="primary"
            htmlType="submit"
            block
            loading={createMutation.isPending || updateMutation.isPending}
          >
            {drawer.editing ? m.usersPage_saveChanges() : m.usersPage_createUser()}
          </Button>
        </Form>
      </Drawer>

      <Drawer
        title={resetTarget ? m.usersPage_resetPasswordFor({ name: resetTarget.name }) : ''}
        open={Boolean(resetTarget)}
        onClose={() => setResetTarget(undefined)}
        size="min(420px, 100vw)"
        destroyOnHidden
      >
        <Form
          form={resetForm}
          layout="vertical"
          requiredMark={false}
          disabled={resetPasswordMutation.isPending}
          onFinish={({ newPassword }) => {
            if (!resetTarget) return
            resetPasswordMutation.mutate(
              { id: resetTarget.id, newPassword },
              { onSuccess: () => setResetTarget(undefined) },
            )
          }}
        >
          <Form.Item
            name="newPassword"
            label={m.auth_newPassword()}
            rules={[{ required: true, min: 8, message: m.auth_passwordRule() }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={resetPasswordMutation.isPending}>
            {m.usersPage_resetPassword()}
          </Button>
        </Form>
      </Drawer>
    </Card>
  )
}
