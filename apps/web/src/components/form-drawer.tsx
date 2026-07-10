import { App, Drawer } from 'antd'
import type { ReactNode } from 'react'

// Drawer wrapper for every form-in-drawer surface: closing (mask click, Esc,
// close button — antd funnels all three through onClose) while the form has
// unsaved edits asks for confirmation instead of silently dropping the user's
// input. A pristine form closes without friction.
export function FormDrawer({
  title,
  open,
  onClose,
  dirty,
  children,
  width = 480,
}: {
  title: ReactNode
  open: boolean
  onClose: () => void
  // Whether the mounted form has unsaved edits; guards the close.
  dirty: boolean
  children: ReactNode
  width?: number
}) {
  const { modal } = App.useApp()

  const guardedClose = () => {
    if (!dirty) {
      onClose()
      return
    }
    modal.confirm({
      title: '放弃未保存的更改？',
      content: '关闭后已填写的内容将丢失',
      okText: '放弃更改',
      okButtonProps: { danger: true },
      cancelText: '继续编辑',
      onOk: onClose,
    })
  }

  return (
    <Drawer
      title={title}
      open={open}
      onClose={guardedClose}
      size={`min(${width}px, 100vw)`}
      destroyOnHidden
    >
      {children}
    </Drawer>
  )
}
