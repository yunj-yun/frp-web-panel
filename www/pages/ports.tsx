import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, Clock3, RefreshCw, Search } from 'lucide-react'
import { Providers } from '@/components/providers'
import { RootLayout } from '@/components/layout'
import { Header } from '@/components/header'
import { ClientSelector } from '@/components/base/client-selector'
import { ServerSelector } from '@/components/base/server-selector'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { getClientsStatus, listNodePortSnapshots } from '@/api/platform'
import { ClientStatus_Status, ListNodePortSnapshotsRequest } from '@/lib/pb/api_master'
import { ClientType, PortSocket } from '@/lib/pb/common'
import { useTranslation } from 'react-i18next'

type NodeKind = 'client' | 'server'
type ProtocolFilter = 'all' | 'tcp' | 'udp'
const emptySockets: PortSocket[] = []

const formatTimestamp = (value: bigint | undefined, locale: string) => {
  if (!value) return '-'
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'medium' }).format(Number(value))
}

function PortsContent() {
  const { t, i18n } = useTranslation()
  const [nodeKind, setNodeKind] = useState<NodeKind>('client')
  const [nodeId, setNodeId] = useState('')
  const [protocol, setProtocol] = useState<ProtocolFilter>('all')
  const [keyword, setKeyword] = useState('')
  const clientType = nodeKind === 'client' ? ClientType.FRPC : ClientType.FRPS

  const portsQuery = useQuery({
    queryKey: ['nodePortSnapshots', nodeKind, nodeId],
    queryFn: () => listNodePortSnapshots(ListNodePortSnapshotsRequest.create({
      clientIds: nodeKind === 'client' ? [nodeId] : [],
      serverIds: nodeKind === 'server' ? [nodeId] : [],
    })),
    enabled: Boolean(nodeId),
    refetchInterval: 30_000,
  })

  const statusQuery = useQuery({
    queryKey: ['nodePortRuntimeStatus', clientType, nodeId],
    queryFn: () => getClientsStatus({ clientType, clientIds: [nodeId] }),
    enabled: Boolean(nodeId),
    refetchInterval: 15_000,
  })

  const snapshot = portsQuery.data?.snapshots.find((item) => item.nodeId === nodeId)
  const sockets = snapshot?.sockets ?? emptySockets
  const filteredSockets = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase()
    return sockets.filter((socket) => {
      if (protocol !== 'all' && socket.protocol.toLowerCase() !== protocol) return false
      if (!normalizedKeyword) return true
      return [socket.address, socket.port, socket.process, socket.pid]
        .some((value) => String(value ?? '').toLowerCase().includes(normalizedKeyword))
    })
  }, [keyword, protocol, sockets])
  const tcpCount = sockets.filter((socket) => socket.protocol.toLowerCase() === 'tcp').length
  const udpCount = sockets.length - tcpCount
  const runtimeStatus = statusQuery.data?.clients[nodeId]?.status
  const isOnline = runtimeStatus === ClientStatus_Status.ONLINE
  const isOffline = runtimeStatus === ClientStatus_Status.OFFLINE

  const refresh = () => {
    void portsQuery.refetch()
    void statusQuery.refetch()
  }

  const renderSocketRow = (socket: PortSocket, index: number) => (
    <TableRow key={`${socket.protocol}-${socket.address}-${socket.port}-${index}`}>
      <TableCell>
        <Badge variant="outline" className="font-mono uppercase">{socket.protocol}</Badge>
      </TableCell>
      <TableCell className="font-mono">{socket.address}</TableCell>
      <TableCell className="font-mono font-medium">{socket.port}</TableCell>
      <TableCell className="max-w-[240px] truncate">{socket.process || t('ports.unknownProcess')}</TableCell>
      <TableCell className="font-mono text-muted-foreground">{socket.pid || '-'}</TableCell>
    </TableRow>
  )

  return (
    <RootLayout mainHeader={<Header />}>
        <main className="mx-auto flex w-full max-w-7xl flex-col gap-6">
          <section className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
            <div className="space-y-1">
              <h1 className="text-2xl font-semibold tracking-tight">{t('ports.title')}</h1>
              <p className="text-sm text-muted-foreground">{t('ports.updatedEvery')}</p>
            </div>
            <Button variant="outline" onClick={refresh} disabled={!nodeId || portsQuery.isFetching || statusQuery.isFetching}>
              <RefreshCw className={portsQuery.isFetching ? 'animate-spin' : ''} />
              {t('refresh.data')}
            </Button>
          </section>

          <section className="flex flex-col gap-3 border-b pb-5 sm:flex-row sm:items-end">
            <div className="space-y-2">
              <span className="text-sm font-medium">{t('ports.nodeType')}</span>
              <div className="flex h-9 rounded-md border bg-muted/40 p-1" role="group" aria-label={t('ports.nodeType')}>
                {(['client', 'server'] as const).map((kind) => (
                  <Button
                    key={kind}
                    type="button"
                    size="sm"
                    variant={nodeKind === kind ? 'secondary' : 'ghost'}
                    aria-pressed={nodeKind === kind}
                    className="h-7 px-3"
                    onClick={() => {
                      setNodeKind(kind)
                      setNodeId('')
                    }}
                  >
                    {t(kind === 'client' ? 'ports.client' : 'ports.server')}
                  </Button>
                ))}
              </div>
            </div>
            <div className="w-full space-y-2 sm:max-w-sm">
              <span className="text-sm font-medium">{t('ports.node')}</span>
              {nodeKind === 'client' ? (
                <ClientSelector clientID={nodeId} setClientID={setNodeId} />
              ) : (
                <ServerSelector serverID={nodeId} setServerID={setNodeId} />
              )}
            </div>
            {nodeId && (
              <div className="flex min-h-9 items-center gap-2 sm:ml-auto">
                <span className={`size-2 rounded-full ${isOnline ? 'bg-emerald-500' : isOffline ? 'bg-muted-foreground/50' : 'bg-amber-500'}`} />
                <span className="text-sm text-muted-foreground">
                  {isOnline ? t('ports.online') : isOffline ? t('ports.offline') : t('ports.checking')}
                </span>
                <span className="font-mono text-sm">{nodeId}</span>
              </div>
            )}
          </section>

          {!nodeId ? (
            <div className="flex min-h-56 flex-col items-center justify-center gap-3 rounded-md border border-dashed text-center">
              <Activity className="size-6 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t('ports.selectNode')}</p>
            </div>
          ) : portsQuery.isPending ? (
            <div className="space-y-3">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-64 w-full" />
            </div>
          ) : portsQuery.isError ? (
            <div className="flex min-h-48 flex-col items-center justify-center gap-3 rounded-md border border-dashed text-center">
              <p className="text-sm text-destructive">{t('ports.loadFailed')}</p>
              <Button variant="outline" size="sm" onClick={refresh}>{t('refresh.data')}</Button>
            </div>
          ) : !snapshot ? (
            <div className="flex min-h-56 flex-col items-center justify-center gap-2 rounded-md border border-dashed text-center">
              <Clock3 className="size-6 text-muted-foreground" />
              <p className="font-medium">{t('ports.noSnapshot')}</p>
              <p className="max-w-md px-4 text-sm text-muted-foreground">{t('ports.noSnapshotHint')}</p>
            </div>
          ) : (
            <>
              <section className="grid grid-cols-2 border-y sm:grid-cols-4">
                <div className="space-y-1 border-b px-4 py-3 sm:border-b-0 sm:border-r">
                  <p className="text-xs text-muted-foreground">{t('ports.totalListeners')}</p>
                  <p className="text-xl font-semibold tabular-nums">{sockets.length}</p>
                </div>
                <div className="space-y-1 border-b px-4 py-3 sm:border-b-0 sm:border-r">
                  <p className="text-xs text-muted-foreground">TCP</p>
                  <p className="text-xl font-semibold tabular-nums">{tcpCount}</p>
                </div>
                <div className="space-y-1 border-r px-4 py-3">
                  <p className="text-xs text-muted-foreground">UDP</p>
                  <p className="text-xl font-semibold tabular-nums">{udpCount}</p>
                </div>
                <div className="space-y-1 px-4 py-3">
                  <p className="text-xs text-muted-foreground">{t('ports.lastScan')}</p>
                  <p className="text-sm font-medium tabular-nums">{formatTimestamp(snapshot.observedAt, i18n.language)}</p>
                </div>
              </section>

              <section className="space-y-3">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex h-9 rounded-md border bg-muted/40 p-1" role="group" aria-label={t('ports.protocol')}>
                    {(['all', 'tcp', 'udp'] as const).map((value) => (
                      <Button
                        key={value}
                        type="button"
                        size="sm"
                        variant={protocol === value ? 'secondary' : 'ghost'}
                        aria-pressed={protocol === value}
                        className="h-7 px-3 uppercase"
                        onClick={() => setProtocol(value)}
                      >
                        {t(`ports.protocols.${value}`)}
                      </Button>
                    ))}
                  </div>
                  <div className="relative w-full sm:max-w-xs">
                    <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      className="pl-9"
                      value={keyword}
                      onChange={(event) => setKeyword(event.target.value)}
                      placeholder={t('ports.search')}
                      aria-label={t('ports.search')}
                    />
                  </div>
                </div>

                <div className="overflow-hidden rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('ports.protocol')}</TableHead>
                        <TableHead>{t('ports.address')}</TableHead>
                        <TableHead>{t('ports.port')}</TableHead>
                        <TableHead>{t('ports.process')}</TableHead>
                        <TableHead>PID</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredSockets.length > 0 ? filteredSockets.map(renderSocketRow) : (
                        <TableRow>
                          <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                            {t('ports.noMatches')}
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                </div>
              </section>
            </>
          )}
        </main>
    </RootLayout>
  )
}

export default function PortsPage() {
  return (
    <Providers>
      <PortsContent />
    </Providers>
  )
}
