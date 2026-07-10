import { create } from '@bufbuild/protobuf'
import {
  BindingCardinality,
  PresentationSchema,
  ProviderDescriptorSchema,
  PurposeDescriptorSchema,
} from '@moonbase/api-client'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { ProfileManager } from '#components/profile-manager'

describe('ProfileManager', () => {
  it('直接展示服务端 purpose 与 provider descriptor', () => {
    const html = renderToStaticMarkup(
      <ProfileManager
        profiles={[{ id: 'p1', name: '主短信', provider: 'aliyun', configValid: false }]}
        bindings={[{ purpose: 'verification', profileIds: ['p1'] }]}
        purposes={[
          create(PurposeDescriptorSchema, {
            key: 'verification',
            presentation: create(PresentationSchema, { name: '短信验证码' }),
            cardinality: BindingCardinality.SINGLE,
          }),
        ]}
        providers={[
          create(ProviderDescriptorSchema, {
            key: 'aliyun',
            presentation: create(PresentationSchema, {
              name: '阿里云短信',
              description: '通过云短信服务发送验证码与通知',
              iconRef: 'antd:MessageOutlined',
            }),
          }),
        ]}
        texts={{
          profilesTitle: '短信配置',
          profilesHint: '配置短信服务',
          noProfiles: '暂无配置',
          confirmDelete: '确认删除？',
          bindingsHint: '选择用途绑定',
        }}
        onAdd={() => undefined}
        onEdit={() => undefined}
        onDelete={() => undefined}
        deleting={false}
        onBind={() => undefined}
        binding={false}
      />,
    )

    expect(html).toContain('短信验证码')
    expect(html).toContain('阿里云短信')
    expect(html).toContain('通过云短信服务发送验证码与通知')
    expect(html).toContain('配置无效')
  })
})
