/**
 * Image workbench gateway client (OpenAI-compatible /v1/images/*)
 */

import { buildGatewayUrl } from './client'

export interface ImageGenerateParams {
  model: string
  prompt: string
  n?: number
  size?: string
  quality?: string
  response_format?: 'url' | 'b64_json'
  output_format?: string
  background?: string
  style?: string
}

export interface ImageGenerateResultItem {
  b64_json?: string
  url?: string
  revised_prompt?: string
}

export interface ImageGenerateResponse {
  created?: number
  data: ImageGenerateResultItem[]
  error?: { message?: string; code?: string }
}

async function parseGatewayError(response: Response): Promise<Error> {
  try {
    const body = await response.json()
    const message = body?.error?.message || body?.message || response.statusText
    const error = new Error(message)
    ;(error as any).code = body?.error?.code || response.status
    ;(error as any).status = response.status
    return error
  } catch {
    const error = new Error(response.statusText || `HTTP ${response.status}`)
    ;(error as any).status = response.status
    return error
  }
}

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKey}`,
    ...extra
  }
}

export async function generateImage(
  apiKey: string,
  params: ImageGenerateParams,
  signal?: AbortSignal
): Promise<ImageGenerateResponse> {
  const payload = {
    model: params.model,
    prompt: params.prompt,
    n: params.n ?? 1,
    size: params.size || '1024x1024',
    quality: params.quality || 'auto',
    response_format: params.response_format || 'b64_json',
    ...(params.output_format ? { output_format: params.output_format } : {}),
    ...(params.background && params.background !== 'auto' ? { background: params.background } : {}),
    ...(params.style && params.style !== 'auto' ? { style: params.style } : {})
  }

  const response = await fetch(buildGatewayUrl('/v1/images/generations'), {
    method: 'POST',
    headers: authHeaders(apiKey, { 'Content-Type': 'application/json' }),
    body: JSON.stringify(payload),
    signal
  })
  if (!response.ok) throw await parseGatewayError(response)
  return response.json()
}

export async function editImage(
  apiKey: string,
  params: {
    model: string
    prompt: string
    imageFile?: File
    imageFiles?: File[]
    n?: number
    size?: string
    quality?: string
    background?: string
    style?: string
    response_format?: 'url' | 'b64_json'
  },
  signal?: AbortSignal
): Promise<ImageGenerateResponse> {
  const files = (params.imageFiles && params.imageFiles.length
    ? params.imageFiles
    : params.imageFile
      ? [params.imageFile]
      : []
  ).filter(Boolean)

  if (!files.length) {
    throw new Error('image file is required')
  }

  const form = new FormData()
  form.append('model', params.model)
  form.append('prompt', params.prompt)
  for (const file of files) {
    // Backend accepts both "image" and "image[...]"; repeat "image" for multi-upload compatibility.
    form.append('image', file, file.name)
  }
  form.append('n', String(params.n ?? 1))
  if (params.size) form.append('size', params.size)
  if (params.quality) form.append('quality', params.quality)
  if (params.background && params.background !== 'auto') form.append('background', params.background)
  if (params.style && params.style !== 'auto') form.append('style', params.style)
  form.append('response_format', params.response_format || 'b64_json')

  const response = await fetch(buildGatewayUrl('/v1/images/edits'), {
    method: 'POST',
    headers: authHeaders(apiKey),
    body: form,
    signal
  })
  if (!response.ok) throw await parseGatewayError(response)
  return response.json()
}

export function resultToDataUrl(item: ImageGenerateResultItem, format = 'png'): string | null {
  if (item.b64_json) {
    const mime = format === 'jpeg' || format === 'jpg' ? 'image/jpeg' : format === 'webp' ? 'image/webp' : 'image/png'
    return `data:${mime};base64,${item.b64_json}`
  }
  if (item.url) return item.url
  return null
}
