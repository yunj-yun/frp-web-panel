import { useMemo } from 'react'
import { useRouter } from 'next/router'
import { useQuery } from '@tanstack/react-query'
import { Activity, Copy, RefreshCw, Server, Terminal, ScrollText, RadioTower } from 'lucide-react'
import { Providers } from '@/components/providers'
import { RootLayout } from '@/components/layout'
import { Header } from '@/components/header'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getClient } from '@/api/client'
import { getServer } from '@/api/server'
import { getPlatformInfo, getClientsStatus, listNodePortSnapshots } from '@/api/platform'
import { Client, ClientType, PortSocket } from '@/lib/pb/common'
import { ClientStatus_Status } from '@/lib/pb/api_master'
import { DockerComposeFile } from '@/lib/consts'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

type NodeKind = 'client' | 'server'

const formatDate = (value: bigint | number | undefined, locale: string) => {
  if (!value) return '-'
  const timestamp = typeof value === 'bigint' ? Number(value) : value
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'medium' }).format(timestamp)
}

function NodeDetailContent() {
  const router = useRouter()
  const { t, i18n } = useTranslation()
  const nodeKind = router.query.nodeType === 'server' ? 'server' : 'client'
  const nodeID = typeof router.query.nodeID === 'string' ? router.query.nodeID : ''
  const clientType = nodeKind === 'client' ? ClientType.FRPC : ClientType.FRPS

  const nodeQuery = useQuery({
    queryKey: ['node-detail', nodeKind, nodeID],
    queryFn: async () => nodeKind === 'client'
      ? (await getClient({ clientId: nodeID })).client
      : (await getServer({ serverId: nodeID })).server,
    enabled: router.isReady && Boolean(nodeID),
  })
  const statusQuery = useQuery({
    queryKey: ['node-detail-status', clientType, nodeID],
    queryFn: () => getClientsStatus({ clientType, clientIds: [nodeID] }),
    enabled: router.isReady && Boolean(nodeID),
    refetchInterval: 15_000,
  })
  const portsQuery = useQuery({
    queryKey: ['node-detail-ports', nodeKind, nodeID],
    queryFn: () => listNodePortSnapshots({
      clientIds: nodeKind === 'client' ? [nodeID] : [],
      serverIds: nodeKind === 'server' ? [nodeID] : [],
    }),
    enabled: router.isReady && Boolean(nodeID),
    refetchInterval: 30_000,
  })
  const platformQuery = useQuery({
    queryKey: ['node-detail-platform'],
    queryFn: getPlatformInfo,
    staleTime: 5 * 60_000,
  })

  const node = nodeQuery.data
  const runtime = statusQuery.data?.clients[nodeID]
  const snapshot = portsQuery.data?.snapshots.find((item) => item.nodeId === nodeID)
  const sockets = useMemo(() => snapshot?.sockets ?? [], [snapshot?.sockets])
  const compose = node && platformQuery.data
    ? DockerComposeFile(nodeKind, { id: node.id || nodeID, secret: node.secret || '' }, platformQuery.data)
    : ''
  const online = runtime?.status === ClientStatus_Status.ONLINE
  const offline = runtime?.status === ClientStatus_Status.OFFLINE
  const statusLabel = online ? t('nodeDetail.online') : offline ? t('nodeDetail.offline') : t('nodeDetail.unknown')
  const statusColor = online ? 'bg-emerald-500' : offline ? 'bg-muted-foreground/50' : 'bg-amber-500'

  const openTool = (page: 'streamlog' | 'console') => {
    void router.push({
      pathname: `/${page}`,
      query: { clientID: nodeID, clientType: clientType.toString() },
    })
  }

  const openEditor = () => {
    void router.push({
      pathname: nodeKind === 'client' ? '/clientedit' : '/serveredit',
      query: nodeKind === 'client' ? { clientID: nodeID } : { serverID: nodeID },
    })
  }

  return (
    <RootLayout mainHeader={<Header />}>
        <main className="mx-auto flex w-full max-w-7xl flex-col gap-6">
          <section className="flex flex-wrap items-end justify-between gap-4">
            <div className="space-y-2">
              <Button variant="link" className="h-auto px-0 text-sm text-muted-foreground" onClick={() => router.back()}>
                {t('nodeDetail.back')}
              </Button>
              <div className="flex items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight">{t(nodeKind === 'client' ? 'nodeDetail.client' : 'nodeDetail.server')}</h1>
                <Badge variant="outline" className="gap-2">
                  <span className={`size-2 rounded-full ${statusColor}`} />{statusLabel}
                </Badge>
              </div>
              <p className="font-mono text-sm text-muted-foreground">{nodeID || t('nodeDetail.missing')}</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => { void nodeQuery.refetch(); void statusQuery.refetch(); void portsQuery.refetch() }}>
                <RefreshCw className="size-4" />{t('nodeDetail.refresh')}
              </Button>
              <Button variant="outline" onClick={() => openTool('streamlog')} disabled={!nodeID}>
                <ScrollText className="size-4" />{t('nodeDetail.logs')}
              </Button>
              <Button variant="outline" onClick={() => openTool('console')} disabled={!nodeID}>
                <Terminal className="size-4" />{t('nodeDetail.terminal')}
              </Button>
              <Button onClick={openEditor} disabled={!nodeID}>
                {t('nodeDetail.edit')}
              </Button>
            </div>
          </section>

          {nodeQuery.isPending ? (
            <div className="grid gap-4 md:grid-cols-2"><Skeleton className="h-48" /><Skeleton className="h-48" /></div>
          ) : nodeQuery.isError || !node ? (
            <Card><CardContent className="flex min-h-48 flex-col items-center justify-center gap-2 text-center">
              <Activity className="size-6 text-muted-foreground" />
              <p className="font-medium">{t('nodeDetail.loadFailed')}</p>
              <Button variant="outline" onClick={() => void nodeQuery.refetch()}>{t('nodeDetail.retry')}</Button>
            </CardContent></Card>
          ) : (
            <>
              <section className="grid gap-4 md:grid-cols-3">
                <Card>
                  <CardHeader className="pb-2"><CardTitle className="text-sm font-medium text-muted-foreground">{t('nodeDetail.connection')}</CardTitle></CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex items-center gap-2"><span className={`size-2.5 rounded-full ${statusColor}`} /><span className="font-medium">{statusLabel}</span></div>
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.address')}: <span className="font-mono text-foreground">{runtime?.addr || '-'}</span></div>
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.latency')}: <span className="font-mono text-foreground">{runtime?.ping != null && runtime.ping >= 0 ? `${runtime.ping} ms` : '-'}</span></div>
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.connectedAt')}: <span className="text-foreground">{formatDate(runtime?.connectTime, i18n.language)}</span></div>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="pb-2"><CardTitle className="text-sm font-medium text-muted-foreground">{t('nodeDetail.agent')}</CardTitle></CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex items-center gap-2"><Server className="size-4 text-muted-foreground" /><span className="font-mono">{runtime?.version?.gitVersion || '-'}</span></div>
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.platform')}: <span className="font-mono text-foreground">{runtime?.version?.platform || '-'}</span></div>
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.goVersion')}: <span className="font-mono text-foreground">{runtime?.version?.goVersion || '-'}</span></div>
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.lastSeen')}: <span className="text-foreground">{formatDate(nodeKind === 'client' ? (node as Client).lastSeenAt : undefined, i18n.language)}</span></div>
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader className="pb-2"><CardTitle className="text-sm font-medium text-muted-foreground">{t('nodeDetail.deployment')}</CardTitle></CardHeader>
                  <CardContent className="space-y-3">
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.network')}: <span className="font-mono text-foreground">{t('nodeDetail.hostNetwork')}</span></div>
                    <div className="text-sm text-muted-foreground">{t('nodeDetail.composeHint')}</div>
                    <details className="rounded-md border p-3">
                      <summary className="cursor-pointer text-sm font-medium">Docker Compose</summary>
                      {compose ? <>
                        <pre className="mt-3 max-h-64 overflow-auto rounded-md bg-muted p-3 font-mono text-xs whitespace-pre-wrap break-all">{compose}</pre>
                        <Button size="sm" variant="outline" className="mt-2" onClick={() => { void navigator.clipboard.writeText(compose); toast(t('nodeDetail.copied')) }}>
                          <Copy className="size-4" />{t('nodeDetail.copyCompose')}
                        </Button>
                      </> : <p className="mt-3 text-sm text-muted-foreground">{t('nodeDetail.composeUnavailable')}</p>}
                    </details>
                  </CardContent>
                </Card>
              </section>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between gap-3">
                  <div><CardTitle className="flex items-center gap-2"><RadioTower className="size-4" />{t('nodeDetail.ports')}</CardTitle><p className="mt-1 text-sm text-muted-foreground">{snapshot ? t('nodeDetail.lastScan', { time: formatDate(snapshot.observedAt, i18n.language) }) : t('nodeDetail.noScan')}</p></div>
                  <Button variant="outline" size="sm" onClick={() => void portsQuery.refetch()} disabled={portsQuery.isFetching}>{t('nodeDetail.refresh')}</Button>
                </CardHeader>
                <CardContent>
                  {portsQuery.isPending ? <Skeleton className="h-32" /> : portsQuery.isError ? <p className="py-8 text-center text-sm text-destructive">{t('nodeDetail.portsFailed')}</p> : sockets.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">{t('nodeDetail.noPorts')}</p> : (
                    <div className="overflow-x-auto">
                      <table className="w-full text-left text-sm">
                        <thead><tr className="border-b text-muted-foreground"><th className="px-3 py-2 font-medium">{t('nodeDetail.protocol')}</th><th className="px-3 py-2 font-medium">{t('nodeDetail.listenAddress')}</th><th className="px-3 py-2 font-medium">{t('nodeDetail.port')}</th><th className="px-3 py-2 font-medium">{t('nodeDetail.process')}</th></tr></thead>
                        <tbody>{sockets.map((socket: PortSocket, index: number) => <tr key={`${socket.protocol}-${socket.address}-${socket.port}-${index}`} className="border-b last:border-0"><td className="px-3 py-2"><Badge variant="outline" className="font-mono uppercase">{socket.protocol}</Badge></td><td className="px-3 py-2 font-mono">{socket.address}</td><td className="px-3 py-2 font-mono font-medium">{socket.port}</td><td className="px-3 py-2 text-muted-foreground">{socket.process || '-'}</td></tr>)}</tbody>
                      </table>
                    </div>
                  )}
                </CardContent>
              </Card>
            </>
          )}
        </main>
      </RootLayout>
  )
}

export default function NodeDetailPage() {
  return (
    <Providers>
      <NodeDetailContent />
    </Providers>
  )
}
