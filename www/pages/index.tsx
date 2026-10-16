import { Providers } from '@/components/providers'
import { RootLayout } from '@/components/layout'
import { Header } from '@/components/header'
import PlatformInfo from '@/components/platforminfo'
import { useTranslation } from 'react-i18next'
import Link from 'next/link'
import { Activity, Cable, MonitorSmartphone, Plus, Server } from 'lucide-react'
import { Button } from '@/components/ui/button'

export default function Home() {
  const { t } = useTranslation()
  return (
    <Providers>
      <RootLayout mainHeader={<Header />}>
        <div className="mx-auto w-full max-w-[1440px] space-y-6 pb-6">
          <section className="flex flex-col gap-4 rounded-lg border bg-card px-5 py-5 shadow-sm sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p className="text-xs font-medium uppercase text-primary/80">{t('dashboard.eyebrow')}</p>
              <h1 className="mt-1 text-2xl font-semibold">{t('dashboard.title')}</h1>
              <p className="mt-1 text-sm text-muted-foreground">{t('dashboard.subtitle')}</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button asChild size="sm">
                <Link href="/clients" prefetch={false}><Plus className="size-4" />{t('dashboard.addClient')}</Link>
              </Button>
              <Button asChild variant="outline" size="sm">
                <Link href="/proxies" prefetch={false}><Cable className="size-4" />{t('dashboard.addTunnel')}</Link>
              </Button>
            </div>
          </section>
          <PlatformInfo />
          <section className="grid gap-3 md:grid-cols-3">
            <Link href="/servers" prefetch={false} className="group flex items-center gap-3 rounded-lg border bg-card px-4 py-3 transition-colors hover:border-primary/40 hover:bg-muted/20">
              <span className="flex size-9 items-center justify-center rounded-md bg-violet-500/10 text-violet-600 dark:text-violet-400"><Server className="size-4" /></span>
              <span><span className="block text-sm font-medium">{t('nav.servers')}</span><span className="text-xs text-muted-foreground">{t('dashboard.manageNodes')}</span></span>
            </Link>
            <Link href="/clients" prefetch={false} className="group flex items-center gap-3 rounded-lg border bg-card px-4 py-3 transition-colors hover:border-primary/40 hover:bg-muted/20">
              <span className="flex size-9 items-center justify-center rounded-md bg-sky-500/10 text-sky-600 dark:text-sky-400"><MonitorSmartphone className="size-4" /></span>
              <span><span className="block text-sm font-medium">{t('nav.clients')}</span><span className="text-xs text-muted-foreground">{t('dashboard.manageNodes')}</span></span>
            </Link>
            <Link href="/clientstats" prefetch={false} className="group flex items-center gap-3 rounded-lg border bg-card px-4 py-3 transition-colors hover:border-primary/40 hover:bg-muted/20">
              <span className="flex size-9 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"><Activity className="size-4" /></span>
              <span><span className="block text-sm font-medium">{t('nav.trafficStats')}</span><span className="text-xs text-muted-foreground">{t('dashboard.viewTraffic')}</span></span>
            </Link>
          </section>
        </div>
      </RootLayout>
    </Providers>
  )
}
