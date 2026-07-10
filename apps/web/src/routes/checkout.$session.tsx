import type { JsonObject } from '@bufbuild/protobuf'
import {
  createQueryOptions,
  useMutation,
  useQuery,
  useSuspenseQuery,
} from '@connectrpc/connect-query'
import {
  type CheckoutOrder,
  confirmCheckout,
  getCheckoutOrder,
  getCheckoutSession,
  type ProviderForm,
  planCheckout,
} from '@moonbase/api-client'
import { createFileRoute } from '@tanstack/react-router'
import { App, Card, Result } from 'antd'
import { useEffect, useRef, useState } from 'react'
import {
  CheckoutSummary,
  isTerminalPaymentStatus,
  PaymentActionView,
} from '#components/payment-checkout'
import { SchemaForm } from '#components/system/config-form'
import { humanizeError } from '#lib/errors'

const POLL_MS = 2_000

export const Route = createFileRoute('/checkout/$session')({
  loader: ({ params, context: { queryClient, transport } }) =>
    queryClient.ensureQueryData(
      createQueryOptions(getCheckoutSession, { session: params.session }, { transport }),
    ),
  component: HostedCheckoutPage,
})

function HostedCheckoutPage() {
  const { message } = App.useApp()
  const { session: token } = Route.useParams()
  const { data } = useSuspenseQuery(getCheckoutSession, { session: token })
  const session = data.session
  const [selectedMethod, setSelectedMethod] = useState(session?.paymentMethod ?? '')
  const [inputForm, setInputForm] = useState<ProviderForm>()
  const [confirmedOrder, setConfirmedOrder] = useState<CheckoutOrder>()
  const restored = useRef(false)

  const planMutation = useMutation(planCheckout, {
    onSuccess: (response) => setInputForm(response.input),
    onError: (error) => message.error(humanizeError(error)),
  })
  const confirmMutation = useMutation(confirmCheckout, {
    onSuccess: (response) => setConfirmedOrder(response.order),
    onError: (error) => message.error(humanizeError(error)),
  })

  useEffect(() => {
    if (!restored.current && session?.paymentMethod && !session.order && !inputForm) {
      restored.current = true
      planMutation.mutate({ session: token, paymentMethod: session.paymentMethod })
    }
  }, [inputForm, planMutation, session, token])

  const currentOrder = confirmedOrder ?? session?.order
  const { data: polled } = useQuery(
    getCheckoutOrder,
    { session: token },
    {
      enabled: Boolean(currentOrder) && !isTerminalPaymentStatus(currentOrder?.order?.status),
      refetchInterval: POLL_MS,
    },
  )
  const checkoutOrder = polled?.order ?? currentOrder

  if (!session) {
    return <Result status="error" title="收银会话不存在" />
  }

  const selectMethod = (method: string) => {
    setSelectedMethod(method)
    setInputForm(undefined)
    planMutation.mutate({ session: token, paymentMethod: method })
  }

  return (
    <div className="min-h-screen bg-(--ant-color-bg-layout) px-4 py-8">
      <div className="mx-auto w-full max-w-lg space-y-4">
        {checkoutOrder ? (
          <Card title={session.subject}>
            <PaymentActionView checkoutOrder={checkoutOrder} returnPath={session.returnPath} />
          </Card>
        ) : session.paymentMethods.length === 0 ? (
          <Result status="warning" title="暂无可用的支付方式" />
        ) : (
          <>
            <CheckoutSummary
              session={session}
              selectedMethod={selectedMethod}
              onSelect={selectMethod}
            />
            {inputForm ? (
              <Card title="支付信息">
                <SchemaForm
                  key={selectedMethod}
                  form={inputForm}
                  saving={confirmMutation.isPending}
                  onSubmit={(inputs: JsonObject) =>
                    confirmMutation.mutate({
                      session: token,
                      paymentMethod: selectedMethod,
                      inputs,
                    })
                  }
                />
              </Card>
            ) : null}
          </>
        )}
      </div>
    </div>
  )
}
