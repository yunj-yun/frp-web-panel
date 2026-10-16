'use client'

import { useMemo, useState } from 'react'
import { CalendarDays, RefreshCw } from 'lucide-react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { listTrafficHistory } from '@/api/stats'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { formatBytes } from '@/lib/utils'
import { ListTrafficHistoryRequest } from '@/lib/pb/api_client'

type RangePreset = '1' | '3' | '7' | '30' | 'custom'

const startOfDay = (value: Date) => {
  const result = new Date(value)
  result.setHours(0, 0, 0, 0)
  return result
}

const dateInputValue = (value: Date) => {
  const year = value.getFullYear()
  const month = String(value.getMonth() + 1).padStart(2, '0')
  const day = String(value.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const dateFromInput = (value: string) => {
  const [year, month, day] = value.split('-').map(Number)
  if (!year || !month || !day) return undefined
  return new Date(year, month - 1, day)
}

const formatChartDate = (value: number, locale: string) => new Intl.DateTimeFormat(locale, { month: 'short', day: 'numeric' }).format(value)

export function TrafficHistoryPanel({ clientID }: { clientID?: string }) {
  const { t, i18n } = useTranslation()
  const today = useMemo(() => startOfDay(new Date()), [])
  const [preset, setPreset] = useState<RangePreset>('7')
  const [customStart, setCustomStart] = useState(dateInputValue(new Date(today.getTime() - 6 * 86400000)))
  const [customEnd, setCustomEnd] = useState(dateInputValue(today))

  const range = useMemo(() => {
    if (preset === 'custom') {
      const start = dateFromInput(customStart)
      const end = dateFromInput(customEnd)
      if (start && end) {
        const exclusiveEnd = new Date(end)
        exclusiveEnd.setDate(exclusiveEnd.getDate() + 1)
        return { start, end: exclusiveEnd }
      }
    }
    const end = startOfDay(new Date())
    const days = Number(preset) || 7
    const start = new Date(end)
    start.setDate(start.getDate() - days + 1)
    const exclusiveEnd = new Date(end)
    exclusiveEnd.setDate(exclusiveEnd.getDate() + 1)
    return { start, end: exclusiveEnd }
  }, [customEnd, customStart, preset])

  const historyQuery = useQuery({
    queryKey: ['trafficHistory', clientID, preset, range.start.getTime(), range.end.getTime()],
    queryFn: () => listTrafficHistory(ListTrafficHistoryRequest.create({
      clientId: clientID,
      startAt: BigInt(range.start.getTime()),
      endAt: BigInt(range.end.getTime()),
    })),
    enabled: Boolean(clientID),
  })

  const chartData = useMemo(() => {
    const grouped = new Map<number, { timestamp: number; trafficIn: number; trafficOut: number }>()
    for (const item of historyQuery.data?.items ?? []) {
      const timestamp = Number(item.statAt)
      const key = new Date(timestamp).setHours(0, 0, 0, 0)
      const current = grouped.get(key) ?? { timestamp: key, trafficIn: 0, trafficOut: 0 }
      current.trafficIn += Number(item.trafficIn)
      current.trafficOut += Number(item.trafficOut)
      grouped.set(key, current)
    }
    return Array.from(grouped.values()).sort((left, right) => left.timestamp - right.timestamp)
      .map((item) => ({ ...item, label: formatChartDate(item.timestamp, i18n.language) }))
  }, [historyQuery.data?.items, i18n.language])

  const totalIn = Number(historyQuery.data?.totalTrafficIn ?? 0n)
  const totalOut = Number(historyQuery.data?.totalTrafficOut ?? 0n)
  const hasCustomRange = preset === 'custom'

  return (
    <Card>
      <CardHeader className="gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle>{t('traffic.historyRange.title')}</CardTitle>
          <CardDescription>{t('traffic.historyRange.description')}</CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Select value={preset} onValueChange={(value) => setPreset(value as RangePreset)}>
            <SelectTrigger className="w-28"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="1">{t('traffic.historyRange.oneDay')}</SelectItem>
              <SelectItem value="3">{t('traffic.historyRange.threeDays')}</SelectItem>
              <SelectItem value="7">{t('traffic.historyRange.sevenDays')}</SelectItem>
              <SelectItem value="30">{t('traffic.historyRange.thirtyDays')}</SelectItem>
              <SelectItem value="custom">{t('traffic.historyRange.custom')}</SelectItem>
            </SelectContent>
          </Select>
          <Button variant="outline" size="icon" onClick={() => void historyQuery.refetch()} disabled={!clientID || historyQuery.isFetching} title={t('refresh.data')}>
            <RefreshCw className={historyQuery.isFetching ? 'animate-spin' : ''} />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {hasCustomRange && (
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="grid gap-1.5 text-sm font-medium">
              <span className="flex items-center gap-1.5"><CalendarDays className="size-4 text-muted-foreground" />{t('traffic.historyRange.start')}</span>
              <Input type="date" value={customStart} onChange={(event) => setCustomStart(event.target.value)} />
            </label>
            <label className="grid gap-1.5 text-sm font-medium">
              <span className="flex items-center gap-1.5"><CalendarDays className="size-4 text-muted-foreground" />{t('traffic.historyRange.end')}</span>
              <Input type="date" value={customEnd} onChange={(event) => setCustomEnd(event.target.value)} />
            </label>
          </div>
        )}
        {!clientID ? (
          <div className="flex min-h-40 items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground">{t('traffic.historyRange.selectClient')}</div>
        ) : historyQuery.isError ? (
          <div className="flex min-h-40 items-center justify-center rounded-md border border-dashed text-sm text-destructive">{t('traffic.historyRange.loadFailed')}</div>
        ) : chartData.length === 0 ? (
          <div className="flex min-h-40 items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground">{t('traffic.historyRange.empty')}</div>
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <div className="rounded-md border px-3 py-2"><p className="text-xs text-muted-foreground">{t('traffic.history.inbound')}</p><p className="font-semibold tabular-nums">{formatBytes(totalIn)}</p></div>
              <div className="rounded-md border px-3 py-2"><p className="text-xs text-muted-foreground">{t('traffic.history.outbound')}</p><p className="font-semibold tabular-nums">{formatBytes(totalOut)}</p></div>
              <div className="rounded-md border px-3 py-2"><p className="text-xs text-muted-foreground">{t('traffic.history.total')}</p><p className="font-semibold tabular-nums">{formatBytes(totalIn + totalOut)}</p></div>
              <div className="rounded-md border px-3 py-2"><p className="text-xs text-muted-foreground">{t('traffic.historyRange.days')}</p><p className="font-semibold tabular-nums">{chartData.length}</p></div>
            </div>
            <div className="h-72 w-full">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 4 }}>
                  <CartesianGrid vertical={false} strokeDasharray="3 3" />
                  <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={20} />
                  <YAxis tickLine={false} axisLine={false} tickFormatter={(value) => formatBytes(Number(value))} width={72} />
                  <Tooltip formatter={(value) => formatBytes(Number(value))} />
                  <Bar dataKey="trafficIn" name={t('traffic.history.inbound')} fill="hsl(var(--chart-1))" radius={[3, 3, 0, 0]} />
                  <Bar dataKey="trafficOut" name={t('traffic.history.outbound')} fill="hsl(var(--chart-2))" radius={[3, 3, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
