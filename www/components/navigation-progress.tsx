import { useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/router'

const SHOW_DELAY_MS = 120

/** Shows a small, non-blocking indicator while a client-side route loads. */
export function NavigationProgress() {
  const router = useRouter()
  const [visible, setVisible] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    const clearTimer = () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
    }

    const handleStart = () => {
      clearTimer()
      timerRef.current = setTimeout(() => setVisible(true), SHOW_DELAY_MS)
    }
    const handleFinish = () => {
      clearTimer()
      setVisible(false)
    }

    router.events.on('routeChangeStart', handleStart)
    router.events.on('routeChangeComplete', handleFinish)
    router.events.on('routeChangeError', handleFinish)

    return () => {
      clearTimer()
      router.events.off('routeChangeStart', handleStart)
      router.events.off('routeChangeComplete', handleFinish)
      router.events.off('routeChangeError', handleFinish)
    }
  }, [router.events])

  if (!visible) return null

  return (
    <div className="pointer-events-none fixed inset-x-0 top-0 z-[100] h-0.5" role="status" aria-label="页面加载中">
      <span className="navigation-progress-bar block h-full w-1/3 rounded-r-full bg-primary shadow-[0_0_12px_hsl(var(--primary)/.7)]" />
      <span className="sr-only">页面加载中</span>
    </div>
  )
}
