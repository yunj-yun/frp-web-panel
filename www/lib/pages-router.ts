import { useMemo } from 'react'
import { useRouter } from 'next/router'

/** Pages Router equivalent of App Router's useSearchParams. */
export function useSearchParams(): Pick<URLSearchParams, 'get'> {
  const router = useRouter()

  return useMemo(() => {
    const queryString = router.asPath.split('?')[1]?.split('#')[0] ?? ''
    return new URLSearchParams(queryString)
  }, [router.asPath])
}
