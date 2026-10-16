import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Activity, CircleAlert, MonitorSmartphone, RefreshCw, Server, ServerCog } from 'lucide-react'
import { useEffect } from 'react'
import { $platformInfo } from '@/store/user'
import { getPlatformInfo } from '@/api/platform'
import { useTranslation } from 'react-i18next';
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export default function PlatformInfo() {
  const { t } = useTranslation();
  const platformInfo = useQuery({
    queryKey: ['platformInfo'],
    queryFn: getPlatformInfo,
    staleTime: 15_000,
  })
  useEffect(() => {
    $platformInfo.set(platformInfo.data)
  }, [platformInfo])

  const metrics = [
    { key: 'configuredServers', value: platformInfo.data?.configuredServerCount, icon: ServerCog, href: '/servers', tone: 'text-emerald-600 dark:text-emerald-400' },
    { key: 'configuredClients', value: platformInfo.data?.configuredClientCount, icon: MonitorSmartphone, href: '/clients', tone: 'text-sky-600 dark:text-sky-400' },
    { key: 'unconfiguredServers', value: platformInfo.data?.unconfiguredServerCount, icon: Server, href: '/servers', tone: 'text-amber-600 dark:text-amber-400' },
    { key: 'unconfiguredClients', value: platformInfo.data?.unconfiguredClientCount, icon: MonitorSmartphone, href: '/clients', tone: 'text-orange-600 dark:text-orange-400' },
    { key: 'totalServers', value: platformInfo.data?.totalServerCount, icon: Server, href: '/servers', tone: 'text-violet-600 dark:text-violet-400' },
    { key: 'totalClients', value: platformInfo.data?.totalClientCount, icon: Activity, href: '/clients', tone: 'text-cyan-600 dark:text-cyan-400' },
  ]

  if (platformInfo.isError) {
    return (
      <section className="flex flex-col items-center justify-center gap-3 rounded-lg border border-destructive/30 bg-destructive/5 px-6 py-10 text-center">
        <CircleAlert className="size-6 text-destructive" />
        <div>
          <p className="font-medium">{t('dashboard.loadFailed')}</p>
          <p className="mt-1 text-sm text-muted-foreground">{t('dashboard.loadFailedHint')}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => platformInfo.refetch()}>
          <RefreshCw className="size-4" />
          {t('refresh.data')}
        </Button>
      </section>
    )
  }

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {metrics.map((metric) => {
        const Icon = metric.icon
        return (
          <Link key={metric.key} href={metric.href} prefetch={false} className="group block">
            <Card className="h-full rounded-lg shadow-none transition-colors duration-200 hover:border-primary/40 hover:bg-muted/20">
              <CardHeader className="flex-row items-start justify-between space-y-0 px-5 pb-2 pt-5">
                <CardTitle className="text-sm font-medium text-muted-foreground">{t(`platform.${metric.key}`)}</CardTitle>
                <Icon className={cn('size-5 transition-transform duration-200 group-hover:scale-110', metric.tone)} />
              </CardHeader>
              <CardContent className="px-5 pb-5">
                {platformInfo.isPending ? (
                  <Skeleton className="h-8 w-16" />
                ) : (
                  <div className="text-2xl font-semibold">{metric.value ?? 0}<span className="ml-1 text-sm font-normal text-muted-foreground">{t('platform.unit')}</span></div>
                )}
                <p className="mt-1 text-xs text-muted-foreground">{t('platform.openDetail')}</p>
              </CardContent>
            </Card>
          </Link>
        )
      })}
    </div>
  )
}
