import { rawJson } from './client'
import type { paths } from './schema'

export type OpenApiMethod = 'get' | 'post' | 'put' | 'patch' | 'delete'

export type MethodsOf<P extends keyof paths> = {
  [M in OpenApiMethod]: M extends keyof paths[P] ? (paths[P][M] extends undefined | never ? never : M) : never
}[OpenApiMethod]

type OperationOf<P extends keyof paths, M extends OpenApiMethod> = M extends keyof paths[P] ? NonNullable<paths[P][M]> : never

type ExtractPathParams<O> = O extends { parameters: { path: infer PP } }
  ? PP
  : O extends { parameters?: { path?: infer PP } }
    ? PP
    : undefined

type ExtractQueryParams<O> = O extends { parameters: { query?: infer QP } }
  ? QP
  : O extends { parameters?: { query?: infer QP } }
    ? QP
    : undefined

type ExtractRequestBody<O> = O extends { requestBody: { content: { 'application/json': infer B } } }
  ? B
  : O extends { requestBody?: { content: { 'application/json': infer B } } }
    ? B
    : undefined

type SuccessStatus = 200 | 201 | 202 | '200' | '201' | '202'
type NoContentStatus = 204 | '204'

type JsonOfResponse<R> = R extends { content: { 'application/json': infer T } }
  ? T
  : R extends { content?: { 'application/json'?: infer T } }
    ? T
    : never

type ExtractJsonResponses<R> = {
  [K in keyof R & SuccessStatus]: JsonOfResponse<R[K]>
}[keyof R & SuccessStatus]

type ExtractVoidResponses<R> = keyof R & NoContentStatus extends never ? never : void

export type OpenApiResponse<O> = O extends { responses: infer R }
  ? [ExtractJsonResponses<R> | ExtractVoidResponses<R>] extends [never]
    ? unknown
    : ExtractJsonResponses<R> | ExtractVoidResponses<R>
  : unknown

export interface OpenApiRequestOptions<O> {
  path?: ExtractPathParams<O> & Record<string, string | number>
  query?: ExtractQueryParams<O> & Record<string, string | number | boolean | null | undefined>
  body?: ExtractRequestBody<O>
}

function resolvePath(template: string, pathParams?: Record<string, string | number>, queryParams?: Record<string, string | number | boolean | null | undefined>): string {
  const interpolated = pathParams
    ? template.replace(/\{([^}]+)\}/g, (_match, key: string) => {
        const val = pathParams[key]
        return val === undefined ? `{${key}}` : encodeURIComponent(String(val))
      })
    : template
  if (!queryParams) return interpolated
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(queryParams)) {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  }
  const qs = sp.toString()
  return qs ? `${interpolated}?${qs}` : interpolated
}

/**
 * Strictly typed OpenAPI request wrapper backed by Stride's CSRF & session cookie transport.
 */
export async function openapiRequest<P extends keyof paths & string, M extends MethodsOf<P>>(
  method: M,
  path: P,
  options?: OpenApiRequestOptions<OperationOf<P, M>>,
): Promise<OpenApiResponse<OperationOf<P, M>>> {
  const url = resolvePath(path, options?.path, options?.query)
  return rawJson<OpenApiResponse<OperationOf<P, M>>>(url, {
    method: method.toUpperCase(),
    body: options?.body,
  })
}
