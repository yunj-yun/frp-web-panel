import {
  SquareTerminal,
  ServerIcon,
  MonitorSmartphoneIcon,
  ChartNetworkIcon,
  Scroll,
  Cable,
  SquareFunction,
  RadioTower,
  ShieldCheck,
  Globe2,
} from "lucide-react"
import { TbBuildingTunnel } from "react-icons/tb"
import { ROUTES } from "@/lib/routes"

export const teams = [
  {
    name: "Frp-Panel",
    logo: TbBuildingTunnel,
    plan: "Community Edition",
    url: "/",
  },
]

export const getNavItems = (t: any) => [
  {
    title: t('nav.clients'),
    url: "/clients",
    icon: MonitorSmartphoneIcon,
    isActive: true,
  },
  {
    title: t('nav.servers'),
    url: "/servers",
    icon: ServerIcon,
  },
  {
    title: t('nav.ports'),
    url: "/ports",
    icon: RadioTower,
  },
  {
    title: t('nav.certificates'),
    url: "/certificates",
    icon: ShieldCheck,
  },
  {
    title: t('nav.gateways'),
    url: "/gateways",
    icon: Globe2,
  },
  {
    title: t('nav.editTunnel'),
    url: "/proxies",
    icon: Cable,
  },
  // {
  //   title: t('nav.editClient'),
  //   url: "/clientedit",
  //   icon: MonitorCogIcon,
  // },
  // {
  //   title: t('nav.editServer'),
  //   url: "/serveredit",
  //   icon: ServerCogIcon,
  // },
  {
    title: t('nav.trafficStats'),
    url: "/clientstats",
    icon: ChartNetworkIcon,
  },
  {
    title: t('nav.realTimeLog'),
    url: "/streamlog",
    icon: Scroll,
  },
  {
    title: t('nav.console'),
    url: "/console",
    icon: SquareTerminal,
  },
  {
    title: t('nav.workers'),
    url: ROUTES.workers,
    icon: SquareFunction,
  },
]
