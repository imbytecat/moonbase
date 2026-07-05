import { createConnectQueryKey, useSuspenseQuery } from '@connectrpc/connect-query'
import { type GetSystemSettingsResponse, getSystemSettings } from '@moonbase/api-client'
import { useQueryClient } from '@tanstack/react-query'

export function useSystemSettings(): {
  data: GetSystemSettingsResponse
  invalidate: () => void
} {
  const queryClient = useQueryClient()
  const { data } = useSuspenseQuery(getSystemSettings)
  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: getSystemSettings, cardinality: 'finite' }),
    })
  }
  return { data, invalidate }
}
