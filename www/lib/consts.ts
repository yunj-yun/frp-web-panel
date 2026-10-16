import * as z from 'zod'
import { Client, Server } from './pb/common'
import { GetPlatformInfoResponse } from './pb/api_user'
import { TypedProxyConfig } from '@/types/proxy'

// 延迟加载前端首选项，避免 SSR 期间引用 window/localStorage
type FrontendPreferenceLazy = {
  githubProxyUrl?: string
  useServerGithubProxyUrl?: boolean
  clientApiUrl?: string
  clientRpcUrl?: string
}

const getFrontendPreference = (): FrontendPreferenceLazy => {
  if (typeof window !== 'undefined') {
    try {
      // 动态引入避免打包时静态依赖
      // eslint-disable-next-line
      const { $frontendPreference } = require('@/store/user')
      return ($frontendPreference.get?.() ?? {}) as FrontendPreferenceLazy
    } catch {
      return {}
    }
  }
  return {}
}

export const API_PATH = '/api/v1'
export const SET_TOKEN_HEADER = 'x-set-authorization'
export const X_CLIENT_REQUEST_ID = 'x-client-request-id'
export const LOCAL_STORAGE_TOKEN_KEY = 'token'
export const ZodPortSchema = z.coerce
  .number({ required_error: 'validation.required' })
  .min(1, { message: 'validation.portRange.min' })
  .max(65535, { message: 'validation.portRange.max' })

export const ZodPortOptionalSchema = z.coerce
  .number({ required_error: 'validation.required' })
  .min(1, { message: 'validation.portRange.min' })
  .max(65535, { message: 'validation.portRange.max' })

export const ZodIPSchema = z
  .string({ required_error: 'validation.required' })
  .regex(/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/, { message: 'validation.ipAddress' })
export const ZodStringSchema = z
  .string({ required_error: 'validation.required' })
  .min(1, { message: 'validation.required' })

export const ZodStringOptionalSchema = z.string().optional()
export const ZodEmailSchema = z
  .string({ required_error: 'validation.required' })
  .min(1, { message: 'validation.required' })
  .email({ message: 'auth.email.invalid' })

export const ConnectionProtocols = ['tcp', 'kcp', 'quic', 'websocket', 'wss']

export const TypedProxyConfigValid = (typedProxyCfg: TypedProxyConfig | undefined): boolean => {
  if (!typedProxyCfg) {
    return false
  }

  if (typedProxyCfg.plugin && typedProxyCfg.plugin.type) {
    if (typedProxyCfg.type === 'tcp' || typedProxyCfg.type === 'udp') {
      if (!typedProxyCfg.remotePort) {
        console.log('remotePort is undefined')
        return false
      }
    }
    return typedProxyCfg.name && typedProxyCfg.type ? true : false
  }

  if (typedProxyCfg.type === 'tcp' || typedProxyCfg.type === 'udp') {
    if (!typedProxyCfg.remotePort) {
      console.log('remotePort is undefined')
      return false
    }
  }

  return typedProxyCfg?.localPort && typedProxyCfg.localIP && typedProxyCfg.name && typedProxyCfg.type ? true : false
}

export const IsIDValid = (clientID: string | undefined): boolean => {
  if (clientID == undefined) {
    return false
  }
  const regex = /^[a-zA-Z0-9-_]+$/
  return clientID.length > 0 && regex.test(clientID)
}

export const ClientConfigured = (client: Client | undefined): boolean => {
  if (client == undefined) {
    return false
  }
  return !(
    (client.config == undefined || client.config == '') &&
    (client.clientIds == undefined || client.clientIds.length == 0)
  )
}

// .refine((e) => e === "abcd@fg.com", "This email is not in our database")

// 获取最终 Github 代理 URL
const getGithubProxyUrl = (info: GetPlatformInfoResponse, applyPref = true): string => {
  const pref = getFrontendPreference()
  if (applyPref && pref.useServerGithubProxyUrl === false && pref.githubProxyUrl) {
    return pref.githubProxyUrl
  }
  // 若前端未指定或选择使用服务器，返回后端
  return info.githubProxyUrl
}

// 获取最终 API URL
const getClientApiUrl = (info: GetPlatformInfoResponse, applyPref = true): string => {
  const pref = getFrontendPreference()
  return applyPref && pref.clientApiUrl?.trim() ? pref.clientApiUrl.trim() : info.clientApiUrl
}

// 获取最终 RPC URL
const getClientRpcUrl = (info: GetPlatformInfoResponse, applyPref = true): string => {
  const pref = getFrontendPreference()
  return applyPref && pref.clientRpcUrl?.trim() ? pref.clientRpcUrl.trim() : info.clientRpcUrl
}

export const ExecCommandStr = <T extends Client | Server>(
  type: 'client' | 'server',
  item: T,
  info: GetPlatformInfoResponse,
  fileName?: string,
  applyPref = true,
) => {
  const apiUrl = getClientApiUrl(info, applyPref)
  const rpcUrl = getClientRpcUrl(info, applyPref)
  return `${fileName || 'frp-panel'} ${type} -s ${item.secret} -i ${item.id} --api-url ${apiUrl} --rpc-url ${rpcUrl}`
}

export const JoinCommandStr = (info: GetPlatformInfoResponse, token: string, fileName?: string, clientID?: string, applyPref = true) => {
  const apiUrl = getClientApiUrl(info, applyPref)
  const rpcUrl = getClientRpcUrl(info, applyPref)
  return `${fileName || 'frp-panel'} join${clientID ? ` -i ${clientID}` : ''} -j ${token} --api-url ${apiUrl} --rpc-url ${rpcUrl}`
}

export const WindowsInstallCommand = <T extends Client | Server>(
  type: 'client' | 'server',
  item: T,
  info: GetPlatformInfoResponse,
  github_proxy?: boolean,
  applyPref = true,
) => {
  const proxyUrl = getGithubProxyUrl(info, applyPref)
  return (
    `[Net.ServicePointManager]::SecurityProtocol = ` +
    `[Net.SecurityProtocolType]::Ssl3 -bor ` +
    `[Net.SecurityProtocolType]::Tls -bor ` +
    `[Net.SecurityProtocolType]::Tls11 -bor ` +
    `[Net.SecurityProtocolType]::Tls12;set-ExecutionPolicy RemoteSigned;` +
    `Invoke-WebRequest ${github_proxy ? proxyUrl : ''}https://raw.githubusercontent.com/VaalaCat/frp-panel/main/install.ps1 ` +
    `-OutFile C:\install.ps1;powershell.exe C:\install.ps1 ${ExecCommandStr(type, item, info, ' ', applyPref)}`
  )
}

export const LinuxInstallCommand = <T extends Client | Server>(
  type: 'client' | 'server',
  item: T,
  info: GetPlatformInfoResponse,
  github_proxy?: boolean,
  applyPref = true,
) => {
  const proxyUrl = getGithubProxyUrl(info, applyPref)
  return `curl -fSL ${github_proxy ? proxyUrl : ''}https://raw.githubusercontent.com/VaalaCat/frp-panel/main/install.sh | bash -s -- ${github_proxy ? `--github-proxy ${proxyUrl}` : ''
    }${ExecCommandStr(type, item, info, ' ', applyPref)}`
}

export const ClientEnvFile = <T extends Client | Server>(item: T, info: GetPlatformInfoResponse, applyPref = true) => {
  const apiUrl = getClientApiUrl(info, applyPref)
  const rpcUrl = getClientRpcUrl(info, applyPref)
  return `CLIENT_ID=${item.id}
CLIENT_SECRET=${item.secret}
CLIENT_API_URL=${apiUrl}
CLIENT_RPC_URL=${rpcUrl}`
}

export const DockerComposeFile = (
  type: 'client' | 'server',
  item: { id: string; secret: string },
  info: GetPlatformInfoResponse,
  applyPref = true,
) => {
  const command = [
    type,
    '-s',
    item.secret,
    '-i',
    item.id,
    '--api-url',
    getClientApiUrl(info, applyPref),
    '--rpc-url',
    getClientRpcUrl(info, applyPref),
  ].map((argument) => argument.replace(/\$/g, '$$$$'))
  const serviceName = type === 'client' ? 'frp-panel-client' : 'frp-panel-server'
  const imageTag = info.imageTag || 'alpha'
  const compose = `services:\n  ${serviceName}:\n    image: vaalacat/frp-panel:${imageTag}\n    container_name: ${serviceName}\n    network_mode: host\n    restart: unless-stopped\n    command: ${JSON.stringify(command)}`

  // Caddy shares the Server host network so it can own the public HTTP/HTTPS
  // listeners directly. The config and storage directories are persisted next
  // to the compose file; the small bootstrap command keeps the generated file
  // runnable before the Server Agent has pushed its first route configuration.
  if (type === 'server') {
    const caddyCommand = [
      '/bin/sh',
      '-ec',
      'mkdir -p /etc/caddy /data /config; if [ ! -s /etc/caddy/Caddyfile ]; then printf \'%s\\n\' \' :80 { respond "Caddy is ready" }\' | sed "s/^ //" > /etc/caddy/Caddyfile; fi; exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile',
    ]
    return `${compose}\n    environment:\n      CADDY_ADMIN_URL: http://127.0.0.1:2019\n      CADDY_DATA_DIR: /data\n    volumes:\n      - ./caddy/data:/data:ro\n  caddy:\n    image: caddy:2-alpine\n    container_name: frp-panel-caddy\n    network_mode: host\n    restart: unless-stopped\n    depends_on:\n      - ${serviceName}\n    command: ${JSON.stringify(caddyCommand)}\n    volumes:\n      - ./caddy:/etc/caddy\n      - ./caddy/data:/data\n      - ./caddy/config:/config`
  }
  return compose
}
