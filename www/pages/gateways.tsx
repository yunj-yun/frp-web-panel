import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Globe2, Pencil, Plus, RefreshCw, Route, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { createGatewayRoute, deleteGatewayRoute, listGatewayRoutes, syncGatewayRoute, updateGatewayRoute } from '@/api/gateway'
import { listManagedCertificates } from '@/api/certificate'
import { listProxyConfig } from '@/api/proxy'
import { Header } from '@/components/header'
import { RootLayout } from '@/components/layout'
import { Providers } from '@/components/providers'
import { ServerSelector } from '@/components/base/server-selector'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import {
  CreateGatewayRouteRequest,
  GatewayRoute,
  ListManagedCertificatesRequest,
  ListGatewayRoutesRequest,
  ManagedCertificate,
  UpdateGatewayRouteRequest,
} from '@/lib/pb/api_certificate'
import { RespCode } from '@/lib/pb/common'

type ProxyOption = { id: number; name: string; config: Record<string, unknown>; stopped: boolean }

const asObject = (value: string | undefined): Record<string, unknown> => {
  if (!value) return {}
  try {
    const parsed = JSON.parse(value) as unknown
    return parsed && typeof parsed === 'object' ? parsed as Record<string, unknown> : {}
  } catch {
    return {}
  }
}

const configDomains = (config: Record<string, unknown>) => {
  const domains = Array.isArray(config.customDomains) ? config.customDomains.filter((item): item is string => typeof item === 'string') : []
  const subdomain = typeof config.subdomain === 'string' ? config.subdomain : ''
  return [...domains, subdomain].filter(Boolean)
}

const statusVariant = (status: string): 'default' | 'secondary' | 'destructive' | 'outline' => {
  if (status === 'active') return 'default'
  if (status === 'error') return 'destructive'
  if (status === 'pending') return 'secondary'
  return 'outline'
}

function GatewayDialog({
  open,
  setOpen,
  route,
  onSaved,
}: {
  open: boolean
  setOpen: (open: boolean) => void
  route?: GatewayRoute
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [domain, setDomain] = useState(route?.domain ?? '')
  const [listenPort, setListenPort] = useState(String(route?.listenPort || 443))
  const [serverId, setServerId] = useState(route?.serverId ?? '')
  const [proxyId, setProxyId] = useState(String(route?.proxyId || ''))
  const [certificateId, setCertificateId] = useState(String(route?.certificateId || ''))

  useEffect(() => {
    if (!open) return
    setDomain(route?.domain ?? '')
    setListenPort(String(route?.listenPort || 443))
    setServerId(route?.serverId ?? '')
    setProxyId(String(route?.proxyId || ''))
    setCertificateId(String(route?.certificateId || ''))
  }, [open, route])

  const proxiesQuery = useQuery({
    queryKey: ['gatewayProxyOptions', serverId],
    queryFn: () => listProxyConfig({ page: 1, pageSize: 100, serverId }),
    enabled: open && Boolean(serverId),
  })
  const certificatesQuery = useQuery({
    queryKey: ['gatewayCertificateOptions', serverId],
    queryFn: () => listManagedCertificates(ListManagedCertificatesRequest.create({ page: 1, pageSize: 100 })),
    enabled: open && Boolean(serverId),
  })
  const proxies = useMemo<ProxyOption[]>(() => (proxiesQuery.data?.proxyConfigs ?? [])
    .filter((item) => item.type === 'http' && item.id && !item.stopped)
    .map((item) => ({ id: item.id!, name: item.name || String(item.id), config: asObject(item.config), stopped: Boolean(item.stopped) })), [proxiesQuery.data])
  const certificates = useMemo<ManagedCertificate[]>(() => (certificatesQuery.data?.certificates ?? []).filter((item) => item.serverId === serverId && item.status !== 'expired' && item.status !== 'error' && item.status !== 'revoked'), [certificatesQuery.data, serverId])
  const selectedProxy = proxies.find((item) => String(item.id) === proxyId)
  const selectedProxyDomains = selectedProxy ? configDomains(selectedProxy.config) : []
  const mutation = useMutation({
    mutationFn: async () => {
      const input = { domain: domain.trim(), listenPort: Number(listenPort) || 443, serverId, proxyId: Number(proxyId), certificateId: Number(certificateId) }
      return route
        ? updateGatewayRoute(UpdateGatewayRouteRequest.create({ id: route.id, ...input }))
        : createGatewayRoute(CreateGatewayRouteRequest.create(input))
    },
    onSuccess: (response) => {
      if (response.status?.code !== RespCode.SUCCESS) {
        toast.error(response.status?.message || t('gateways.operationFailed'))
        return
      }
      toast.success(t('gateways.saved'))
      setOpen(false)
      onSaved()
    },
    onError: (error) => toast.error(t('gateways.operationFailed'), { description: error instanceof Error ? error.message : String(error) }),
  })
  const canSave = Boolean(domain.trim() && serverId && proxyId && certificateId && Number(listenPort) >= 1 && Number(listenPort) <= 65535)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{route ? t('gateways.edit') : t('gateways.create')}</DialogTitle>
          <DialogDescription>{t('gateways.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="gateway-domain">{t('gateways.domain')}</label>
            <Input id="gateway-domain" value={domain} onChange={(event) => setDomain(event.target.value)} placeholder="app.example.com" />
          </div>
          <div className="grid gap-2 sm:grid-cols-[1fr_8rem]">
            <div className="grid gap-2">
              <label className="text-sm font-medium">{t('gateways.server')}</label>
              <ServerSelector serverID={serverId} setServerID={(value) => { setServerId(value); setProxyId(''); setCertificateId('') }} />
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="gateway-port">{t('gateways.listenPort')}</label>
              <Input id="gateway-port" type="number" min={1} max={65535} value={listenPort} onChange={(event) => setListenPort(event.target.value)} />
            </div>
          </div>
          <div className="grid gap-2">
            <label className="text-sm font-medium">{t('gateways.httpTunnel')}</label>
            <Select value={proxyId} onValueChange={setProxyId} disabled={!serverId || proxiesQuery.isLoading}>
              <SelectTrigger><SelectValue placeholder={t('gateways.selectTunnel')} /></SelectTrigger>
              <SelectContent>
                {proxies.map((proxy) => <SelectItem key={proxy.id} value={String(proxy.id)}>{proxy.name} · #{proxy.id}</SelectItem>)}
              </SelectContent>
            </Select>
            {selectedProxyDomains.length > 0 && <p className="text-xs text-muted-foreground">{t('gateways.tunnelDomains')}: {selectedProxyDomains.join(', ')}</p>}
            {serverId && !proxiesQuery.isLoading && proxies.length === 0 && <p className="text-xs text-destructive">{t('gateways.noHttpTunnel')}</p>}
          </div>
          <div className="grid gap-2">
            <label className="text-sm font-medium">{t('gateways.certificate')}</label>
            <Select value={certificateId} onValueChange={setCertificateId} disabled={!serverId || certificatesQuery.isLoading}>
              <SelectTrigger><SelectValue placeholder={t('gateways.selectCertificate')} /></SelectTrigger>
              <SelectContent>
                {certificates.map((certificate) => <SelectItem key={certificate.id} value={String(certificate.id)}>{certificate.primaryDomain} · {certificate.status}</SelectItem>)}
              </SelectContent>
            </Select>
            {serverId && !certificatesQuery.isLoading && certificates.length === 0 && <p className="text-xs text-destructive">{t('gateways.noCertificate')}</p>}
          </div>
        </div>
        <DialogFooter>
          <Button onClick={() => mutation.mutate()} disabled={!canSave || mutation.isPending}>
            {mutation.isPending && <RefreshCw className="animate-spin" />}
            {t('gateways.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function GatewaysContent() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<GatewayRoute>()
  const routesQuery = useQuery({
    queryKey: ['gatewayRoutes', keyword],
    queryFn: () => listGatewayRoutes(ListGatewayRoutesRequest.create({ page: 1, pageSize: 100, keyword })),
    refetchInterval: 15_000,
  })
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['gatewayRoutes'] })
  const actionMutation = useMutation({
    mutationFn: ({ action, id }: { action: 'delete' | 'sync'; id: number }) => action === 'delete' ? deleteGatewayRoute({ id }) : syncGatewayRoute({ id }),
    onSuccess: (response) => {
      if (response.status?.code !== RespCode.SUCCESS) toast.error(response.status?.message || t('gateways.operationFailed'))
      else { toast.success(t('gateways.updated')); refresh() }
    },
    onError: (error) => toast.error(t('gateways.operationFailed'), { description: error instanceof Error ? error.message : String(error) }),
  })
  const routes = routesQuery.data?.routes ?? []
  const formatDate = (value: bigint) => value ? new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium', timeStyle: 'short' }).format(Number(value)) : '-'

  return (
    <RootLayout mainHeader={<Header />}>
      <main className="mx-auto flex w-full max-w-7xl flex-col gap-6">
        <section className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
          <div className="space-y-1">
            <div className="flex items-center gap-2"><Globe2 className="size-5 text-primary" /><h1 className="text-2xl font-semibold tracking-tight">{t('gateways.title')}</h1></div>
            <p className="text-sm text-muted-foreground">{t('gateways.description')}</p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={refresh} disabled={routesQuery.isFetching}><RefreshCw className={routesQuery.isFetching ? 'animate-spin' : ''} />{t('refresh.data')}</Button>
            <Button onClick={() => { setEditing(undefined); setDialogOpen(true) }}><Plus />{t('gateways.create')}</Button>
          </div>
        </section>
        <section className="flex items-start gap-3 rounded-md border border-primary/20 bg-primary/5 p-4 text-sm"><Route className="mt-0.5 size-4 shrink-0 text-primary" /><p className="text-muted-foreground">{t('gateways.notice')}</p></section>
        <Input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder={t('input.keyword.placeholder')} className="max-w-sm" />
        {routesQuery.isError ? <div className="flex min-h-48 items-center justify-center rounded-md border border-dashed text-sm text-destructive">{t('gateways.loadFailed')}</div> : routesQuery.isPending ? <div className="space-y-3"><Skeleton className="h-12 w-full" /><Skeleton className="h-64 w-full" /></div> : routes.length === 0 ? <div className="flex min-h-56 flex-col items-center justify-center gap-2 rounded-md border border-dashed text-center"><Globe2 className="size-6 text-muted-foreground" /><p className="font-medium">{t('gateways.empty')}</p><p className="text-sm text-muted-foreground">{t('gateways.emptyHint')}</p><Button variant="outline" size="sm" onClick={() => { setEditing(undefined); setDialogOpen(true) }}><Plus />{t('gateways.create')}</Button></div> : (
          <div className="overflow-hidden rounded-md border"><Table><TableHeader><TableRow><TableHead>{t('gateways.domain')}</TableHead><TableHead>{t('gateways.server')}</TableHead><TableHead>{t('gateways.listenPort')}</TableHead><TableHead>{t('gateways.upstream')}</TableHead><TableHead>{t('gateways.status')}</TableHead><TableHead>{t('gateways.lastApplied')}</TableHead><TableHead className="text-right">{t('common.actions')}</TableHead></TableRow></TableHeader><TableBody>{routes.map((item) => <TableRow key={item.id}><TableCell><div className="font-medium">{item.domain}</div><div className="text-xs text-muted-foreground">HTTPS</div></TableCell><TableCell className="font-mono text-sm">{item.serverId}</TableCell><TableCell className="font-mono">:{item.listenPort}</TableCell><TableCell className="font-mono text-sm">127.0.0.1:{item.upstreamPort}</TableCell><TableCell><Badge variant={statusVariant(item.status)}>{t(`gateways.status${item.status.charAt(0).toUpperCase()}${item.status.slice(1)}`, { defaultValue: item.status })}</Badge>{item.errorMessage && <div className="mt-1 max-w-48 truncate text-xs text-destructive" title={item.errorMessage}>{item.errorMessage}</div>}</TableCell><TableCell className="text-sm text-muted-foreground">{formatDate(item.lastAppliedAt)}</TableCell><TableCell><div className="flex justify-end gap-1"><Button variant="ghost" size="icon" title={t('gateways.sync')} disabled={actionMutation.isPending} onClick={() => actionMutation.mutate({ action: 'sync', id: item.id })}><RefreshCw className="size-4" /></Button><Button variant="ghost" size="icon" title={t('gateways.edit')} onClick={() => { setEditing(item); setDialogOpen(true) }}><Pencil className="size-4" /></Button><Button variant="ghost" size="icon" title={t('gateways.delete')} disabled={actionMutation.isPending} onClick={() => { if (window.confirm(t('gateways.confirmDelete'))) actionMutation.mutate({ action: 'delete', id: item.id }) }}><Trash2 className="size-4 text-destructive" /></Button></div></TableCell></TableRow>)}</TableBody></Table></div>
        )}
        <GatewayDialog open={dialogOpen} setOpen={setDialogOpen} route={editing} onSaved={refresh} />
      </main>
    </RootLayout>
  )
}

export default function Gateways() {
  return <Providers><GatewaysContent /></Providers>
}
