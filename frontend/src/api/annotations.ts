import client from './client'

export type AnnotationTargetType = 'trace' | 'service'

export interface Annotation {
  id: string
  target_type: AnnotationTargetType
  target_id: string
  kind: string
  payload: Record<string, unknown>
  ts: string
  created_at: string
}

export interface CreateAnnotationInput {
  target_type: AnnotationTargetType
  target_id: string
  kind: string
  payload: Record<string, unknown>
  ts: string
}

// Uses the shared axios client (api/client.ts) so the Bearer interceptor runs.
// Raw fetch() here would skip auth — the exact SLICE-3 T15 regression.
export async function fetchAnnotations(
  targetType: AnnotationTargetType,
  opts: { targetId?: string; limit?: number } = {},
): Promise<Annotation[]> {
  const params: Record<string, string | number> = { target_type: targetType }
  if (opts.targetId) params.target_id = opts.targetId
  if (opts.limit) params.limit = opts.limit
  const { data } = await client.get<Annotation[]>('/v1/annotations', { params })
  return data
}

export async function createAnnotation(input: CreateAnnotationInput): Promise<string> {
  const { data } = await client.post<{ annotation_id: string }>('/v1/annotations', input)
  return data.annotation_id
}
