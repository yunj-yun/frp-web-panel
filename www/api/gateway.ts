import http from '@/api/http'
import { API_PATH } from '@/lib/consts'
import {
  CreateGatewayRouteRequest,
  CreateGatewayRouteResponse,
  DeleteGatewayRouteRequest,
  DeleteGatewayRouteResponse,
  ListGatewayRoutesRequest,
  ListGatewayRoutesResponse,
  SyncGatewayRouteRequest,
  SyncGatewayRouteResponse,
  UpdateGatewayRouteRequest,
  UpdateGatewayRouteResponse,
} from '@/lib/pb/api_certificate'
import { BaseResponse } from '@/types/api'

export const listGatewayRoutes = async (req: ListGatewayRoutesRequest) => {
  const res = await http.post(API_PATH + '/gateway-route/list', ListGatewayRoutesRequest.toJson(req))
  return ListGatewayRoutesResponse.fromJson((res.data as BaseResponse).body)
}

export const createGatewayRoute = async (req: CreateGatewayRouteRequest) => {
  const res = await http.post(API_PATH + '/gateway-route/create', CreateGatewayRouteRequest.toJson(req))
  return CreateGatewayRouteResponse.fromJson((res.data as BaseResponse).body)
}

export const updateGatewayRoute = async (req: UpdateGatewayRouteRequest) => {
  const res = await http.post(API_PATH + '/gateway-route/update', UpdateGatewayRouteRequest.toJson(req))
  return UpdateGatewayRouteResponse.fromJson((res.data as BaseResponse).body)
}

export const deleteGatewayRoute = async (req: DeleteGatewayRouteRequest) => {
  const res = await http.post(API_PATH + '/gateway-route/delete', DeleteGatewayRouteRequest.toJson(req))
  return DeleteGatewayRouteResponse.fromJson((res.data as BaseResponse).body)
}

export const syncGatewayRoute = async (req: SyncGatewayRouteRequest) => {
  const res = await http.post(API_PATH + '/gateway-route/sync', SyncGatewayRouteRequest.toJson(req))
  return SyncGatewayRouteResponse.fromJson((res.data as BaseResponse).body)
}
