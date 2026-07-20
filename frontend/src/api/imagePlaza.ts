/**
 * Global Image Plaza APIs (JWT session)
 */

import { apiClient } from './client'

export interface ImagePlazaItem {
  id: number
  user_id: number
  user_email?: string
  prompt: string
  title: string
  model: string
  size: string
  quality: string
  format: string
  background: string
  style: string
  content_type: string
  file_size: number
  visibility: string
  image_url: string
  created_at: string
}

export interface ImagePlazaListResponse {
  items: ImagePlazaItem[]
  total: number
  page: number
  page_size: number
}

export interface PublishImagePlazaPayload {
  prompt: string
  title?: string
  model: string
  size?: string
  quality?: string
  format?: string
  background?: string
  style?: string
  image: string // data URL or base64
}

export async function listImagePlaza(params?: {
  q?: string
  page?: number
  page_size?: number
}): Promise<ImagePlazaListResponse> {
  const { data } = await apiClient.get<ImagePlazaListResponse>('/image-plaza', { params })
  return data
}

export async function publishImagePlaza(payload: PublishImagePlazaPayload): Promise<ImagePlazaItem> {
  const { data } = await apiClient.post<ImagePlazaItem>('/image-plaza', payload)
  return data
}

export async function deleteImagePlaza(id: number): Promise<void> {
  await apiClient.delete(`/image-plaza/${id}`)
}

export function resolvePlazaImageUrl(item: Pick<ImagePlazaItem, 'id' | 'image_url'>): string {
  if (item.image_url) return item.image_url
  return `/api/v1/image-plaza/${item.id}/content`
}
