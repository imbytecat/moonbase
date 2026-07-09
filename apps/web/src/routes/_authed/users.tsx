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
      message.success('用户已创建')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const updateMutation = useMutation(updateUser, {
    onSuccess: () => {
      void invalidate()
      setDrawer({ open: false })
      message.success('用户已更新')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const deleteMutation = useMutation(deleteUser, {
    onSuccess: () => {
      void invalidate()
      message.success('用户已删除')
    },
    onError: (err) => message.error(humanizeError(err)),
  })

  const resetPasswordMutation = useMutation(resetUserPassword, {
    onSuccess: () => message.success('密码已重置，该用户的所有会话已退出'),
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
      title={'用户管理'}
      extra={
        canWrite ? (
          <Button type="primary" onClick={openCreate}>
            {'添加用户'}
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
            title: '用户',
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
            title: '角色',
            dataIndex: 'roles',
            render: (roles: string[]) =>
              roles.length ? roles.map((r) => <Tag key={r}>{r}</Tag>) : <Tag>{'无角色'}</Tag>,
          },
          {
            title: '状态',
            dataIndex: 'isActive',
            render: (active: boolean) =>
              active ? <Tag color="green">{'正常'}</Tag> : <Tag color="red">{'禁用'}</Tag>,
          },
          {
            title: '创建时间',
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
                        {'编辑'}
                      </Button>
                      <Button
                        size="small"
                        onClick={() => {
                          resetForm.resetFields()
                          setResetTarget(u)
                        }}
                      >
                        {'重置密码'}
                      </Button>
                      <Popconfirm
                        title={'删除该用户？'}
                        okText={'删除'}
                        okButtonProps={{ danger: true }}
                        onConfirm={() => deleteMutation.mutate({ id: u.id })}
                      >
                        <Button size="small" danger>
                          {'删除'}
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
        title={drawer.editing ? `编辑 ${drawer.editing.name}` : '添加用户'}
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
          <Form.Item name="name" label={'姓名'} rules={[{ required: true, message: '请输入姓名' }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="username"
            label={'用户名'}
            dependencies={['email']}
            rules={[
              {
                pattern: /^[a-zA-Z][a-zA-Z0-9._-]{2,31}$/,
                message: '3-32 位，字母开头，可含字母、数字、. _ -',
              },
              ({ getFieldValue }) => ({
                validator: () =>
                  getFieldValue('username') || getFieldValue('email')
                    ? Promise.resolve()
                    : Promise.reject(new Error('用户名和邮箱至少填一项')),
              }),
            ]}
          >
            <Input disabled={Boolean(drawer.editing)} />
          </Form.Item>
          <Form.Item
            name="email"
            label={'邮箱'}
            dependencies={['username']}
            rules={[
              { type: 'email', message: '请输入有效的邮箱地址' },
              ({ getFieldValue }) => ({
                validator: () =>
                  getFieldValue('username') || getFieldValue('email')
                    ? Promise.resolve()
                    : Promise.reject(new Error('用户名和邮箱至少填一项')),
              }),
            ]}
          >
            <Input />
          </Form.Item>
          {drawer.editing ? null : (
            <Form.Item
              name="password"
              label={'密码'}
              rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
          )}
          <Form.Item name="roleIds" label={'角色'}>
            <Select mode="multiple" options={roleOptions} placeholder={'分配角色'} />
          </Form.Item>
          <Form.Item name="isActive" label={'启用'} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Button
            type="primary"
            htmlType="submit"
            block
            loading={createMutation.isPending || updateMutation.isPending}
          >
            {drawer.editing ? '保存修改' : '创建用户'}
          </Button>
        </Form>
      </Drawer>

      <Drawer
        title={resetTarget ? `重置 ${resetTarget.name} 的密码` : ''}
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
            label={'新密码'}
            rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={resetPasswordMutation.isPending}>
            {'重置密码'}
          </Button>
        </Form>
      </Drawer>
    </Card>
  )
}
