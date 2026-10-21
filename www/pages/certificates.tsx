import { useCallback, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, History, Pencil, Plus, RefreshCw, ScrollText, ShieldCheck, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { createManagedCertificate, deleteManagedCertificate, listCertificateJobs, listManagedCertificates, requestCertificate, updateManagedCertificate } from '@/api/certificate'
import { Header } from '@/components/header'
import { RootLayout } from '@/components/layout'
import { Providers } from '@/components/providers'
import { ServerSelector } from '@/components/base/server-selector'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import {
  CreateManagedCertificateRequest,
  ListCertificateJobsRequest,
  ListManagedCertificatesRequest,
  ManagedCertificate,
  RequestCertificateRequest,
  UpdateManagedCertificateRequest,
} from '@/lib/pb/api_certificate'
import { RespCode } from '@/lib/pb/common'

const parseDomains = (value: string) => value.split(/[\n,]/).map((domain) => domain.trim()).filter(Boolean)

const formatDate = (value: bigint, fallback: string, locale: string) => {
  if (!value) return fallback
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(Number(value))
}

const statusVariant = (status: string): 'default' | 'secondary' | 'destructive' | 'outline' => {
  if (status === 'ready') return 'default'
  if (status === 'error' || status === 'expired' || status === 'revoked') return 'destructive'
  if (status === 'renewing') return 'secondary'
  return 'outline'
}

const statusLabel = (translate: (key: string, options?: { defaultValue?: string }) => string, status: string) => {
  const key = status ? `certificates.status${status.charAt(0).toUpperCase()}${status.slice(1)}` : 'certificates.statusPending'
  return translate(key, { defaultValue: status || translate('certificates.statusPending') })
}

function CertificateDialog({
  open,
  setOpen,
  certificate,
  onSaved,
}: {
  open: boolean
  setOpen: (open: boolean) => void
  certificate?: ManagedCertificate
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(certificate?.name ?? '')
  const [domains, setDomains] = useState(certificate?.domains.join('\n') ?? '')
  const [serverID, setServerID] = useState(certificate?.serverId ?? '')
  const [challenge, setChallenge] = useState(certificate?.challenge || 'http-01')
  const [autoRenew, setAutoRenew] = useState(certificate?.autoRenew ?? true)

  const createMutation = useMutation({ mutationFn: createManagedCertificate })
  const updateMutation = useMutation({ mutationFn: updateManagedCertificate })
  const isPending = createMutation.isPending || updateMutation.isPending

  const resetForOpen = useCallback(() => {
    setName(certificate?.name ?? '')
    setDomains(certificate?.domains.join('\n') ?? '')
    setServerID(certificate?.serverId ?? '')
    setChallenge(certificate?.challenge || 'http-01')
    setAutoRenew(certificate?.autoRenew ?? true)
  }, [certificate])

  useEffect(() => {
    if (open) resetForOpen()
  }, [open, resetForOpen])

  const save = async () => {
    const domainValues = parseDomains(domains)
    if (domainValues.length === 0 || !serverID) {
      toast.error(t('certificates.operationFailed'), { description: t('certificates.emptyHint') })
      return
    }
    try {
      const response = certificate
        ? await updateMutation.mutateAsync(UpdateManagedCertificateRequest.create({
            id: certificate.id,
            name,
            domains: domainValues,
            serverId: serverID,
            challenge,
            autoRenew,
          }))
        : await createMutation.mutateAsync(CreateManagedCertificateRequest.create({
            name,
            domains: domainValues,
            serverId: serverID,
            challenge,
            autoRenew,
          }))
      if (response.status?.code !== RespCode.SUCCESS) {
        throw new Error(response.status?.message || t('certificates.operationFailed'))
      }
      toast.success(t('certificates.saveSuccess'))
      setOpen(false)
      onSaved()
    } catch (error) {
      toast.error(t('certificates.operationFailed'), { description: error instanceof Error ? error.message : String(error) })
    }
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => {
      if (nextOpen) resetForOpen()
      setOpen(nextOpen)
    }}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{certificate ? t('certificates.edit') : t('certificates.create')}</DialogTitle>
          <DialogDescription>{t('certificates.description')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="certificate-name">{t('certificates.name')}</label>
            <Input id="certificate-name" value={name} onChange={(event) => setName(event.target.value)} placeholder={t('certificates.namePlaceholder')} />
          </div>
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="certificate-domains">{t('certificates.domains')}</label>
            <Textarea id="certificate-domains" value={domains} onChange={(event) => setDomains(event.target.value)} placeholder={t('certificates.domainsPlaceholder')} rows={4} />
          </div>
          <div className="grid gap-2">
            <label className="text-sm font-medium">{t('certificates.server')}</label>
            <ServerSelector serverID={serverID} setServerID={setServerID} />
          </div>
          <div className="grid gap-2">
            <label className="text-sm font-medium">{t('certificates.challenge')}</label>
            <Select value={challenge} onValueChange={setChallenge}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="http-01">{t('certificates.http01')}</SelectItem>
                <SelectItem value="tls-alpn-01">{t('certificates.tlsAlpn01')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={autoRenew} onCheckedChange={(checked) => setAutoRenew(checked === true)} />
            {t('certificates.autoRenew')}
          </label>
        </div>
        <DialogFooter>
          <Button onClick={save} disabled={isPending || !serverID || parseDomains(domains).length === 0}>
            {isPending && <RefreshCw className="animate-spin" />}
            {t('certificates.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

const jobStatusVariant = (status: string): 'default' | 'secondary' | 'destructive' | 'outline' => {
  if (status === 'success') return 'default'
  if (status === 'failed') return 'destructive'
  if (status === 'running') return 'secondary'
  return 'outline'
}

const jobStatusLabel = (translate: (key: string, options?: { defaultValue?: string }) => string, status: string) => {
  const key = status ? `certificates.jobStatus${status.charAt(0).toUpperCase()}${status.slice(1)}` : 'certificates.jobStatusQueued'
  return translate(key, { defaultValue: status || translate('certificates.jobStatusQueued') })
}

const jobActionLabel = (translate: (key: string, options?: { defaultValue?: string }) => string, action: string) => {
  const key = action ? `certificates.jobAction${action.charAt(0).toUpperCase()}${action.slice(1)}` : ''
  return key ? translate(key, { defaultValue: action }) : action
}

function CertificateJobsDialog({
  open,
  setOpen,
  certificate,
}: {
  open: boolean
  setOpen: (open: boolean) => void
  certificate?: ManagedCertificate
}) {
  const { t, i18n } = useTranslation()
  const jobsQuery = useQuery({
    queryKey: ['certificateJobs', certificate?.id],
    queryFn: () => listCertificateJobs(ListCertificateJobsRequest.create({ certificateId: certificate?.id ?? 0, page: 1, pageSize: 50 })),
    enabled: open && Boolean(certificate?.id),
    refetchInterval: open ? 10_000 : false,
  })
  const jobs = useMemo(() => jobsQuery.data?.jobs ?? [], [jobsQuery.data?.jobs])
  const formatTimestamp = (value: bigint) => value ? new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium', timeStyle: 'medium' }).format(Number(value)) : '-'

  // Caddy runs on the target Server and writes its ACME errors to its data
  // directory. The Server log stream shows the raw Caddy output, which is the
  // fastest way to see why an ACME challenge failed.
  const openServerLogs = () => {
    if (!certificate?.serverId) return
    window.open(`/streamlog?clientID=${encodeURIComponent(certificate.serverId)}&clientType=2`, '_blank', 'noopener')
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('certificates.jobHistoryTitle')}</DialogTitle>
          <DialogDescription>
            {certificate ? `${certificate.primaryDomain} · ${t('certificates.jobHistoryDescription')}` : t('certificates.jobHistoryDescription')}
          </DialogDescription>
        </DialogHeader>
        {certificate && (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="text-muted-foreground">{t('certificates.status')}:</span>
            <Badge variant={statusVariant(certificate.status)}>{statusLabel(t, certificate.status)}</Badge>
            <span className="text-muted-foreground">{t('certificates.expiresAt')}: {formatDate(certificate.expiresAt, t('certificates.notIssued'), i18n.language)}</span>
          </div>
        )}
        <div className="grid gap-3">
          {certificate?.errorMessage && (
            <div className="flex items-start gap-3 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm">
              <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
              <div className="space-y-1">
                <p className="font-medium text-destructive">{t('certificates.currentError')}</p>
                <pre className="max-h-32 overflow-auto whitespace-pre-wrap break-all font-mono text-xs text-destructive">{certificate.errorMessage}</pre>
                <p className="text-xs text-muted-foreground">{t('certificates.jobHistoryHint')}</p>
              </div>
            </div>
          )}
          <div className="flex items-center justify-between gap-2">
            {/* Reuse the existing hint as the ACME troubleshooting guidance next
                to the error details; otherwise keep it above the history table. */}
            {!certificate?.errorMessage && <p className="text-xs text-muted-foreground">{t('certificates.jobHistoryHint')}</p>}
            <Button className="ml-auto" variant="outline" size="sm" onClick={() => void jobsQuery.refetch()} disabled={jobsQuery.isFetching}>
              <RefreshCw className={jobsQuery.isFetching ? 'animate-spin' : ''} />
              {t('refresh.data')}
            </Button>
          </div>
          {jobsQuery.isError ? (
            <div className="flex min-h-32 flex-col items-center justify-center gap-3 rounded-md border border-dashed text-center">
              <p className="text-sm text-destructive">{t('certificates.jobHistoryLoadFailed')}</p>
              <Button variant="outline" size="sm" onClick={() => void jobsQuery.refetch()}>{t('refresh.data')}</Button>
            </div>
          ) : jobsQuery.isPending ? (
            <div className="space-y-2"><Skeleton className="h-10 w-full" /><Skeleton className="h-32 w-full" /></div>
          ) : jobs.length === 0 ? (
            <div className="flex min-h-32 flex-col items-center justify-center gap-2 rounded-md border border-dashed text-center">
              <History className="size-5 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t('certificates.jobHistoryEmpty')}</p>
            </div>
          ) : (
            <div className="max-h-[26rem] overflow-auto rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('certificates.jobAction')}</TableHead>
                    <TableHead>{t('certificates.jobStatusColumn')}</TableHead>
                    <TableHead>{t('certificates.jobAttempts')}</TableHead>
                    <TableHead>{t('certificates.jobCreatedAt')}</TableHead>
                    <TableHead>{t('certificates.jobFinishedAt')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.map((job) => (
                    <TableRow key={job.id}>
                      <TableCell className="align-top text-sm">{jobActionLabel(t, job.action)}</TableCell>
                      <TableCell className="align-top">
                        <Badge variant={jobStatusVariant(job.status)}>{jobStatusLabel(t, job.status)}</Badge>
                        {job.errorMessage && (
                          <pre className="mt-2 max-h-40 max-w-md overflow-auto whitespace-pre-wrap break-all rounded bg-destructive/5 p-2 font-mono text-xs text-destructive">{job.errorMessage}</pre>
                        )}
                      </TableCell>
                      <TableCell className="align-top font-mono text-sm">{job.attempts}</TableCell>
                      <TableCell className="align-top text-sm text-muted-foreground">{formatTimestamp(job.createdAt)}</TableCell>
                      <TableCell className="align-top text-sm text-muted-foreground">{formatTimestamp(job.finishedAt)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
        <DialogFooter className={certificate?.serverId ? 'flex-col sm:flex-row sm:justify-between' : undefined}>
          {certificate?.serverId && (
            <Button variant="outline" onClick={openServerLogs}>
              <ScrollText className="size-4" />
              {t('certificates.viewServerLogs')}
            </Button>
          )}
          <Button variant="outline" onClick={() => setOpen(false)}>{t('certificates.close')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CertificatesContent() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<ManagedCertificate>()
  const [jobsDialogOpen, setJobsDialogOpen] = useState(false)
  const [jobsCertificate, setJobsCertificate] = useState<ManagedCertificate>()

  const certificatesQuery = useQuery({
    queryKey: ['managedCertificates', keyword],
    queryFn: () => listManagedCertificates(ListManagedCertificatesRequest.create({ page: 1, pageSize: 100, keyword })),
  })

  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['managedCertificates'] })
  const requestMutation = useMutation({
    mutationFn: (certificateID: number) => requestCertificate(RequestCertificateRequest.create({ certificateId: certificateID })),
    onSuccess: (response) => {
      if (response.status?.code === RespCode.SUCCESS) {
        toast.success(t('certificates.requestQueued'))
        refresh()
      } else {
        toast.error(t('certificates.operationFailed'), { description: response.status?.message })
      }
    },
    onError: (error) => toast.error(t('certificates.operationFailed'), { description: error instanceof Error ? error.message : String(error) }),
  })
  const deleteMutation = useMutation({
    mutationFn: (certificateID: number) => deleteManagedCertificate({ id: certificateID }),
    onSuccess: (response) => {
      if (response.status?.code === RespCode.SUCCESS) {
        toast.success(t('common.success'))
        refresh()
      } else {
        toast.error(t('certificates.operationFailed'), { description: response.status?.message })
      }
    },
    onError: (error) => toast.error(t('certificates.operationFailed'), { description: error instanceof Error ? error.message : String(error) }),
  })

  const certificates = useMemo(() => certificatesQuery.data?.certificates ?? [], [certificatesQuery.data?.certificates])

  const openCreate = () => {
    setEditing(undefined)
    setDialogOpen(true)
  }

  const openEdit = (certificate: ManagedCertificate) => {
    setEditing(certificate)
    setDialogOpen(true)
  }

  const openJobs = (certificate: ManagedCertificate) => {
    setJobsCertificate(certificate)
    setJobsDialogOpen(true)
  }

  return (
    <RootLayout mainHeader={<Header />}>
      <main className="mx-auto flex w-full max-w-7xl flex-col gap-6">
        <section className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <ShieldCheck className="size-5 text-primary" />
              <h1 className="text-2xl font-semibold tracking-tight">{t('certificates.title')}</h1>
            </div>
            <p className="text-sm text-muted-foreground">{t('certificates.description')}</p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={refresh} disabled={certificatesQuery.isFetching}>
              <RefreshCw className={certificatesQuery.isFetching ? 'animate-spin' : ''} />
              {t('refresh.data')}
            </Button>
            <Button onClick={openCreate}><Plus />{t('certificates.create')}</Button>
          </div>
        </section>

        <section className="flex items-start gap-3 rounded-md border border-primary/20 bg-primary/5 p-4 text-sm">
          <AlertCircle className="mt-0.5 size-4 shrink-0 text-primary" />
          <p className="text-muted-foreground">{t('certificates.pendingNotice')}</p>
        </section>

        <Input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder={t('input.keyword.placeholder')} className="max-w-sm" />

        {certificatesQuery.isError ? (
          <div className="flex min-h-48 flex-col items-center justify-center gap-3 rounded-md border border-dashed text-center">
            <p className="text-sm text-destructive">{t('certificates.loadFailed')}</p>
            <Button variant="outline" size="sm" onClick={() => void certificatesQuery.refetch()}>{t('refresh.data')}</Button>
          </div>
        ) : certificatesQuery.isPending ? (
          <div className="space-y-3"><Skeleton className="h-12 w-full" /><Skeleton className="h-64 w-full" /></div>
        ) : certificates.length === 0 ? (
          <div className="flex min-h-56 flex-col items-center justify-center gap-2 rounded-md border border-dashed text-center">
            <ShieldCheck className="size-6 text-muted-foreground" />
            <p className="font-medium">{t('certificates.empty')}</p>
            <p className="max-w-md px-4 text-sm text-muted-foreground">{t('certificates.emptyHint')}</p>
            <Button variant="outline" size="sm" onClick={openCreate}><Plus />{t('certificates.create')}</Button>
          </div>
        ) : (
          <div className="overflow-hidden rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('certificates.domain')}</TableHead>
                  <TableHead>{t('certificates.serverColumn')}</TableHead>
                  <TableHead>{t('certificates.challengeColumn')}</TableHead>
                  <TableHead>{t('certificates.status')}</TableHead>
                  <TableHead>{t('certificates.expiresAt')}</TableHead>
                  <TableHead className="text-right">{t('common.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {certificates.map((certificate) => (
                  <TableRow key={certificate.id}>
                    <TableCell>
                      <div className="font-medium">{certificate.primaryDomain}</div>
                      {certificate.domains.length > 1 && <div className="text-xs text-muted-foreground">+{certificate.domains.length - 1} SAN</div>}
                    </TableCell>
                    <TableCell className="font-mono text-sm">{certificate.serverId}</TableCell>
                    <TableCell className="text-sm">{certificate.challenge}</TableCell>
                    <TableCell>
                      <Badge variant={statusVariant(certificate.status)}>{statusLabel(t, certificate.status)}</Badge>
                      {certificate.errorMessage && (
                        <div className="mt-1 max-w-56 truncate text-xs text-destructive" title={certificate.errorMessage}>{certificate.errorMessage}</div>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{formatDate(certificate.expiresAt, t('certificates.notIssued'), i18n.language)}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="icon" title={t('certificates.edit')} onClick={() => openEdit(certificate)}><Pencil className="size-4" /></Button>
                        <Button variant="ghost" size="icon" title={t('certificates.request')} disabled={requestMutation.isPending} onClick={() => requestMutation.mutate(certificate.id)}><RefreshCw className="size-4" /></Button>
                        <Button variant="ghost" size="icon" title={t('certificates.viewRequests')} onClick={() => openJobs(certificate)}><History className="size-4" /></Button>
                        <Button variant="ghost" size="icon" title={t('certificates.delete')} disabled={deleteMutation.isPending} onClick={() => {
                          if (window.confirm(t('certificates.confirmDelete'))) deleteMutation.mutate(certificate.id)
                        }}><Trash2 className="size-4 text-destructive" /></Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <CertificateDialog open={dialogOpen} setOpen={setDialogOpen} certificate={editing} onSaved={refresh} />
        <CertificateJobsDialog open={jobsDialogOpen} setOpen={setJobsDialogOpen} certificate={jobsCertificate} />
      </main>
    </RootLayout>
  )
}

export default function CertificatesPage() {
  return (
    <Providers>
      <CertificatesContent />
    </Providers>
  )
}
