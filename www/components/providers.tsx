import React from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  SidebarProvider,
} from "@/components/ui/sidebar"
import { Toaster } from './ui/sonner'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

export const Providers = ({ children }: { children: React.ReactNode }) => {
  return (
      <SidebarProvider>
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        <Toaster />
      </SidebarProvider>
  )
}
