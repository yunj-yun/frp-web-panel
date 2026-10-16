import http from '@/api/http'
import { API_PATH } from '@/lib/consts'
import {
  CreateManagedCertificateRequest,
  CreateManagedCertificateResponse,
  DeleteManagedCertificateRequest,
  DeleteManagedCertificateResponse,
  ListCertificateJobsRequest,
  ListCertificateJobsResponse,
  ListManagedCertificatesRequest,
  ListManagedCertificatesResponse,
  RequestCertificateRequest,
  RequestCertificateResponse,
  UpdateManagedCertificateRequest,
  UpdateManagedCertificateResponse,
} from '@/lib/pb/api_certificate'
import { BaseResponse } from '@/types/api'

export const listManagedCertificates = async (req: ListManagedCertificatesRequest) => {
  const res = await http.post(API_PATH + '/certificate/list', ListManagedCertificatesRequest.toJson(req))
  return ListManagedCertificatesResponse.fromJson((res.data as BaseResponse).body)
}

export const createManagedCertificate = async (req: CreateManagedCertificateRequest) => {
  const res = await http.post(API_PATH + '/certificate/create', CreateManagedCertificateRequest.toJson(req))
  return CreateManagedCertificateResponse.fromJson((res.data as BaseResponse).body)
}

export const updateManagedCertificate = async (req: UpdateManagedCertificateRequest) => {
  const res = await http.post(API_PATH + '/certificate/update', UpdateManagedCertificateRequest.toJson(req))
  return UpdateManagedCertificateResponse.fromJson((res.data as BaseResponse).body)
}

export const deleteManagedCertificate = async (req: DeleteManagedCertificateRequest) => {
  const res = await http.post(API_PATH + '/certificate/delete', DeleteManagedCertificateRequest.toJson(req))
  return DeleteManagedCertificateResponse.fromJson((res.data as BaseResponse).body)
}

export const requestCertificate = async (req: RequestCertificateRequest) => {
  const res = await http.post(API_PATH + '/certificate/request', RequestCertificateRequest.toJson(req))
  return RequestCertificateResponse.fromJson((res.data as BaseResponse).body)
}

export const listCertificateJobs = async (req: ListCertificateJobsRequest) => {
  const res = await http.post(API_PATH + '/certificate/jobs', ListCertificateJobsRequest.toJson(req))
  return ListCertificateJobsResponse.fromJson((res.data as BaseResponse).body)
}
