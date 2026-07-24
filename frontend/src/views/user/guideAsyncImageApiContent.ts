export interface ApiParam {
  name: string
  required: boolean
  type: string
  desc: string
}

export interface ApiEndpointBlock {
  id: string
  title: string
  method: 'POST' | 'GET'
  path: string
  summary: string
  contentType?: string
  params: ApiParam[]
  bodyExample: string
  notes?: string[]
}

export interface ApiDocSection {
  id: string
  title: string
  intro?: string
  endpoints?: ApiEndpointBlock[]
  extraHtml?: string
}

export function getAsyncImageApiDoc(locale: string, apiRoot: string) {
  const root = apiRoot.replace(/\/+$/, '')
  const v1 = /\/v1$/i.test(root) ? root : `${root}/v1`
  const isEn = locale.toLowerCase().startsWith('en')

  if (isEn) {
    return {
      title: 'Async Image API',
      subtitle:
        'Submit text-to-image or image-to-image jobs, get a task id, poll status, and receive OSS URLs valid for 1 day.',
      baseUrl: root,
      toc: [
        { id: 'overview', label: 'Overview', level: 1 },
        { id: 'auth', label: 'Auth', level: 1 },
        { id: 'openai', label: 'OpenAI image groups', level: 1 },
        { id: 'openai-t2i', label: 'Text-to-image', level: 2 },
        { id: 'openai-i2i', label: 'Image-to-image', level: 2 },
        { id: 'gemini', label: 'Gemini image groups', level: 1 },
        { id: 'gemini-t2i', label: 'Text-to-image', level: 2 },
        { id: 'gemini-i2i', label: 'Image-to-image', level: 2 },
        { id: 'query', label: 'Query status', level: 1 },
        { id: 'oss', label: 'OSS links', level: 1 },
      ],
      overview: {
        bullets: [
          'Only OpenAI / Gemini image groups with async image generation enabled.',
          'OpenAI accept: HTTP 202 + task_id. Gemini (SC) accept: HTTP 200 + data.id.',
          'On success, result URLs are OSS links valid for 1 day — download promptly.',
          'Poll with the same API key that submitted the job (OpenAI: tasks_async; Gemini: tasks_sc).',
        ],
      },
      auth: {
        header: 'Authorization: Bearer YOUR_API_KEY',
        contentType: 'Content-Type: application/json',
        idempotency: 'Optional Idempotency-Key (≤255 bytes) to avoid duplicate billing on retries.',
      },
      openaiT2I: {
        id: 'openai-t2i',
        title: 'OpenAI · Text-to-image',
        method: 'POST' as const,
        path: `${v1}/images/generations_oa`,
        summary: 'Create images from a text prompt. No reference images.',
        contentType: 'application/json',
        params: [
          { name: 'model', required: true, type: 'string', desc: 'Image model available for the group.' },
          { name: 'prompt', required: true, type: 'string', desc: 'Text description of the image.' },
          { name: 'n', required: false, type: 'number', desc: 'Number of images (model-dependent).' },
          { name: 'resolution', required: false, type: 'string', desc: '1K / 2K / 4K preferred.' },
          { name: 'aspect_ratio', required: false, type: 'string', desc: '1:1, 3:2, 2:3, 16:9, 9:16.' },
          { name: 'size', required: false, type: 'string', desc: 'Compat: ratio (9:16), WxH (1024x1024), auto, or 2K as resolution.' },
          { name: 'quality', required: false, type: 'string', desc: 'e.g. high / medium / low (model-dependent).' },
          { name: 'background', required: false, type: 'string', desc: 'Background option if the model supports it.' },
          { name: 'output_format', required: false, type: 'string', desc: 'png / jpeg / webp when supported.' },
          { name: 'stream', required: false, type: 'boolean', desc: 'Must be false or omitted.' },
        ],
        bodyExample: `{
  "model": "gpt-image-2",
  "prompt": "a cat on the beach, photorealistic",
  "n": 1,
  "resolution": "1K",
  "aspect_ratio": "3:2",
  "quality": "high"
}`,
        acceptExample: `{
  "task_id": "asyncimg_0123456789abcdef",
  "query_url": "${v1}/images/tasks_async/asyncimg_0123456789abcdef"
}`,
      },
      openaiI2I: {
        id: 'openai-i2i',
        title: 'OpenAI · Image-to-image',
        method: 'POST' as const,
        path: `${v1}/images/edits_oa`,
        summary: 'Edit / restyle with reference images. JSON (image_urls) or multipart file upload.',
        contentType: 'application/json  or  multipart/form-data',
        params: [
          { name: 'model', required: true, type: 'string', desc: 'Image model available for the group.' },
          { name: 'prompt', required: true, type: 'string', desc: 'Edit instruction.' },
          { name: 'image_urls', required: true, type: 'string[]', desc: 'Required in JSON mode: HTTPS reference URLs.' },
          { name: 'image', required: true, type: 'file', desc: 'Required in multipart mode: form field name image.' },
          { name: 'resolution', required: false, type: 'string', desc: '1K / 2K / 4K.' },
          { name: 'aspect_ratio', required: false, type: 'string', desc: 'Same as text-to-image; auto when refs allow.' },
          { name: 'mask.image_url', required: false, type: 'string', desc: 'Optional mask URL for masked edits.' },
        ],
        bodyExample: `{
  "model": "gpt-image-2",
  "prompt": "keep subject, change background to night",
  "image_urls": [
    "https://cdn.example.com/reference.png"
  ],
  "resolution": "1K",
  "aspect_ratio": "1:1"
}`,
        acceptExample: `{
  "task_id": "asyncimg_0123456789abcdef",
  "query_url": "${v1}/images/tasks_async/asyncimg_0123456789abcdef"
}`,
        notes: [
          'JSON uses image_urls (string array). Legacy images[].image_url is also accepted.',
          'images[].file_id is not supported.',
          'Multipart: -F model=... -F prompt=... -F image=@file.png',
        ],
      },
      geminiT2I: {
        id: 'gemini-t2i',
        title: 'Gemini · Text-to-image',
        method: 'POST' as const,
        path: `${v1}/images/generations_sc`,
        summary: 'Simple JSON body. Omit image_urls for text-to-image. Accept: HTTP 200.',
        contentType: 'application/json',
        params: [
          { name: 'model', required: true, type: 'string', desc: 'Gemini image model mapped for the group.' },
          { name: 'prompt', required: true, type: 'string', desc: 'Text description of the image.' },
          { name: 'resolution', required: false, type: 'string', desc: '1K / 2K / 4K.' },
          { name: 'size', required: false, type: 'string', desc: 'Aspect ratio alias: 1:1, 3:2, 16:9, … Or tier 2K when resolution is empty.' },
          { name: 'aspect_ratio', required: false, type: 'string', desc: 'Same ratios as size. auto only when image_urls is present.' },
        ],
        bodyExample: `{
  "model": "gemini-3-pro-image-preview",
  "prompt": "modern living room, nordic style, soft daylight",
  "resolution": "2K",
  "size": "16:9"
}`,
        acceptExample: `{
  "code": 200,
  "message": "success",
  "data": {
    "id": "asyncimg_0123456789abcdef",
    "status": "pending",
    "type": "image",
    "progress": 0
  }
}`,
        notes: [
          'Query with GET /v1/tasks_sc/{task_id} using the same API key.',
        ],
      },
      geminiI2I: {
        id: 'gemini-i2i',
        title: 'Gemini · Image-to-image',
        method: 'POST' as const,
        path: `${v1}/images/generations_sc`,
        summary: 'Same path as text-to-image. Pass one or more reference URLs in image_urls. Accept: HTTP 200.',
        contentType: 'application/json',
        params: [
          { name: 'model', required: true, type: 'string', desc: 'Gemini image model.' },
          { name: 'prompt', required: true, type: 'string', desc: 'Edit instruction (can refer to 图1 / 图2 order).' },
          { name: 'image_urls', required: true, type: 'string[]', desc: 'HTTPS reference image URLs (PNG / JPG / WEBP).' },
          { name: 'resolution', required: false, type: 'string', desc: '1K / 2K / 4K.' },
          { name: 'size', required: false, type: 'string', desc: 'Aspect ratio, e.g. 3:2. Equivalent to aspect_ratio.' },
          { name: 'aspect_ratio', required: false, type: 'string', desc: 'Optional; size is preferred by many clients. auto allowed with references.' },
        ],
        bodyExample: `{
  "image_urls": [
    "https://cdn.example.com/ref-1.jpg",
    "https://cdn.example.com/ref-2.jpg"
  ],
  "model": "gemini-3-pro-image-preview",
  "prompt": "clean matte gallery tone, keep all native textures of bamboo, wood, ceramic and fabric, soft warm light matching reference image 2",
  "resolution": "4K"
}`,
        acceptExample: `{
  "code": 200,
  "message": "success",
  "data": {
    "id": "asyncimg_0123456789abcdef",
    "status": "pending",
    "type": "image",
    "progress": 0
  }
}`,
        notes: [
          'image_urls order is 图1, 图2, … in the prompt.',
          'size / aspect_ratio are optional; omit to use upstream default ratio.',
          'Query: GET /v1/tasks_sc/{id} — success status is completed with result.images[].url (OSS, 1 day).',
        ],
      },
      query: {
        id: 'query',
        title: 'Query task status',
        method: 'GET' as const,
        path: `${v1}/images/tasks_async/{task_id}`,
        geminiPath: `${v1}/tasks_sc/{task_id}`,
        summary: 'Poll every 30–60 seconds with the same API key. OpenAI uses tasks_async; Gemini uses tasks_sc. HTTP 200 does not mean success — check status.',
        statuses: [
          { status: 'queued / pending', meaning: 'Accepted, waiting for worker' },
          { status: 'processing', meaning: 'Upstream / upload / billing in progress' },
          { status: 'succeeded / completed', meaning: 'Done — read OSS URLs (valid 1 day)' },
          { status: 'failed', meaning: 'Terminal failure' },
        ],
        queuedExample: `{
  "status": "queued",
  "task_id": "asyncimg_0123456789abcdef"
}`,
        successExample: `{
  "status": "succeeded",
  "task_id": "asyncimg_0123456789abcdef",
  "data": [
    { "url": "https://oss.example.com/images/results/output-1.png" }
  ]
}`,
        failedExample: `{
  "status": "failed",
  "task_id": "asyncimg_0123456789abcdef",
  "fail_reason": "image generation failed"
}`,
      },
      oss: {
        title: 'OSS result links',
        bullets: [
          'Only use data[].url when status is succeeded.',
          'OSS links are valid for 1 day — download or re-host before expiry.',
          'Do not share API keys; poll only with the submitting key.',
        ],
      },
      labels: {
        required: 'Required',
        optional: 'Optional',
        params: 'Parameters',
        requestBody: 'Request body',
        acceptResponse: 'Accept response (202)',
        statusTable: 'Status values',
        notes: 'Notes',
        baseUrl: 'Base URL',
        copy: 'Copy',
        copied: 'Copied',
      },
    }
  }

  return {
    title: '异步生图 API 文档',
    subtitle:
      '支持文生图 / 图生图 API 调用：提交返回任务号，轮询状态查询接口；成功后返回 OSS 图片链接（有效期 1 天）。',
    baseUrl: root,
    toc: [
      { id: 'overview', label: '概览', level: 1 },
      { id: 'auth', label: '鉴权', level: 1 },
      { id: 'openai', label: 'OpenAI 系列生图分组', level: 1 },
      { id: 'openai-t2i', label: '文生图', level: 2 },
      { id: 'openai-i2i', label: '图生图', level: 2 },
      { id: 'gemini', label: 'Gemini 系列生图分组', level: 1 },
      { id: 'gemini-t2i', label: '文生图', level: 2 },
      { id: 'gemini-i2i', label: '图生图', level: 2 },
      { id: 'query', label: '任务状态查询', level: 1 },
      { id: 'oss', label: 'OSS 链接说明', level: 1 },
    ],
    overview: {
      bullets: [
        '仅「已开启异步生图」的 OpenAI / Gemini 系列的生图分组可用。',
        'OpenAI 受理：HTTP 202 + task_id；Gemini（SC）受理：HTTP 200 + data.id。',
        '成功后结果 URL 为 OSS 链接，有效期 1 天，请及时下载转存。',
        '查询必须使用提交该任务的同一个 API Key（OpenAI：tasks_async；Gemini：tasks_sc）。',
      ],
    },
    auth: {
      header: 'Authorization: Bearer 你的_API_Key',
      contentType: 'Content-Type: application/json',
      idempotency: '可选 Idempotency-Key（≤255 字节），避免网络重试导致重复计费。',
    },
    openaiT2I: {
      id: 'openai-t2i',
      title: 'OpenAI · 文生图',
      method: 'POST' as const,
      path: `${v1}/images/generations_oa`,
      summary: '仅文本提示词生成图片，不携带参考图。',
      contentType: 'application/json',
      params: [
        { name: 'model', required: true, type: 'string', desc: '分组可用的图片模型名。' },
        { name: 'prompt', required: true, type: 'string', desc: '画面描述提示词。' },
        { name: 'n', required: false, type: 'number', desc: '生成张数（受模型能力限制）。' },
        { name: 'resolution', required: false, type: 'string', desc: '推荐：1K / 2K / 4K。' },
        { name: 'aspect_ratio', required: false, type: 'string', desc: '1:1、3:2、2:3、16:9、9:16。' },
        { name: 'size', required: false, type: 'string', desc: '兼容字段：比例（如 9:16）、WxH（如 1024x1024）、auto，或档位（如 2K）。' },
        { name: 'quality', required: false, type: 'string', desc: '如 high / medium / low（视模型支持）。' },
        { name: 'background', required: false, type: 'string', desc: '背景相关参数（视模型支持）。' },
        { name: 'output_format', required: false, type: 'string', desc: '输出格式：png / jpeg / webp（视模型支持）。' },
        { name: 'stream', required: false, type: 'boolean', desc: '必须为 false 或省略，异步接口不支持流式。' },
      ],
      bodyExample: `{
  "model": "gpt-image-2",
  "prompt": "一只在沙滩上的猫，写实风格",
  "n": 1,
  "resolution": "1K",
  "aspect_ratio": "3:2",
  "quality": "high"
}`,
      acceptExample: `{
  "task_id": "asyncimg_0123456789abcdef",
  "query_url": "${v1}/images/tasks_async/asyncimg_0123456789abcdef"
}`,
    },
    openaiI2I: {
      id: 'openai-i2i',
      title: 'OpenAI · 图生图',
      method: 'POST' as const,
      path: `${v1}/images/edits_oa`,
      summary: '带参考图改图。支持 JSON（image_urls）或 multipart 文件上传。',
      contentType: 'application/json  或  multipart/form-data',
      params: [
        { name: 'model', required: true, type: 'string', desc: '分组可用的图片模型名。' },
        { name: 'prompt', required: true, type: 'string', desc: '改图指令。' },
        { name: 'image_urls', required: true, type: 'string[]', desc: 'JSON 模式必填：参考图 HTTPS URL 数组。' },
        { name: 'image', required: true, type: 'file', desc: 'multipart 模式必填：表单文件字段名 image。' },
        { name: 'resolution', required: false, type: 'string', desc: '1K / 2K / 4K。' },
        { name: 'aspect_ratio', required: false, type: 'string', desc: '同文生图；有参考图时可按模型支持使用 auto。' },
        { name: 'mask.image_url', required: false, type: 'string', desc: '可选遮罩图 URL（局部编辑）。' },
      ],
      bodyExample: `{
  "model": "gpt-image-2",
  "prompt": "保留主体，把背景换成夜景",
  "image_urls": [
    "https://cdn.example.com/reference.png"
  ],
  "resolution": "1K",
  "aspect_ratio": "1:1"
}`,
      acceptExample: `{
  "task_id": "asyncimg_0123456789abcdef",
  "query_url": "${v1}/images/tasks_async/asyncimg_0123456789abcdef"
}`,
      notes: [
        'JSON 使用 image_urls（字符串数组）；也兼容旧字段 images[].image_url。',
        '不支持 images[].file_id。',
        'multipart 示例：-F model=... -F prompt=... -F image=@reference.png',
        '参考图格式建议：PNG / JPG / WEBP。',
      ],
    },
    geminiT2I: {
      id: 'gemini-t2i',
      title: 'Gemini · 文生图',
      method: 'POST' as const,
      path: `${v1}/images/generations_sc`,
      summary: '简洁 JSON 请求体。文生图不传 image_urls。受理响应 HTTP 200。',
      contentType: 'application/json',
      params: [
        { name: 'model', required: true, type: 'string', desc: '分组映射的 Gemini 图片模型。' },
        { name: 'prompt', required: true, type: 'string', desc: '图片描述提示词。' },
        { name: 'resolution', required: false, type: 'string', desc: '1K / 2K / 4K。' },
        { name: 'size', required: false, type: 'string', desc: '比例别名：1:1、3:2、16:9 等；未传 resolution 时也可写 2K 表示清晰度。' },
        { name: 'aspect_ratio', required: false, type: 'string', desc: '与 size 同系列比例。文生图不可用 auto。' },
      ],
      bodyExample: `{
  "model": "gemini-3-pro-image-preview",
  "prompt": "现代客厅，北欧风，自然光",
  "resolution": "2K",
  "size": "16:9"
}`,
      acceptExample: `{
  "code": 200,
  "message": "success",
  "data": {
    "id": "asyncimg_0123456789abcdef",
    "status": "pending",
    "type": "image",
    "progress": 0
  }
}`,
      notes: [
        '查询请使用 GET /v1/tasks_sc/{task_id}，与提交使用同一 API Key。',
      ],
    },
    geminiI2I: {
      id: 'gemini-i2i',
      title: 'Gemini · 图生图',
      method: 'POST' as const,
      path: `${v1}/images/generations_sc`,
      summary: '与文生图同一路径。通过 image_urls 传入一张或多张参考图。受理响应 HTTP 200。',
      contentType: 'application/json',
      params: [
        { name: 'model', required: true, type: 'string', desc: 'Gemini 图片模型。' },
        { name: 'prompt', required: true, type: 'string', desc: '编辑说明（可按图1、图2 顺序引用参考图）。' },
        { name: 'image_urls', required: true, type: 'string[]', desc: '参考图公网 HTTPS URL（PNG / JPG / WEBP）。' },
        { name: 'resolution', required: false, type: 'string', desc: '1K / 2K / 4K。' },
        { name: 'size', required: false, type: 'string', desc: '宽高比，例如 3:2；与 aspect_ratio 等价。' },
        { name: 'aspect_ratio', required: false, type: 'string', desc: '可选；多数客户端优先用 size。有参考图时可用 auto。' },
      ],
      bodyExample: `{
  "image_urls": [
    "https://cdn.example.com/ref-1.jpg",
    "https://cdn.example.com/ref-2.jpg"
  ],
  "model": "gemini-3-pro-image-preview",
  "prompt": "画面通透干净，哑光高级展厅室内色调，完整保留竹编、实木、陶瓷、布艺全部原生肌理，光影柔和温润，视觉氛围与参考图二商业茶室柔和质感完全统一。",
  "resolution": "4K"
}`,
      acceptExample: `{
  "code": 200,
  "message": "success",
  "data": {
    "id": "asyncimg_0123456789abcdef",
    "status": "pending",
    "type": "image",
    "progress": 0
  }
}`,
      notes: [
        'image_urls 顺序对应提示词中的图1、图2…',
        'size / aspect_ratio 可选；不传则使用上游默认比例。',
        '查询：GET /v1/tasks_sc/{id}；成功状态为 completed，结果在 result.images[].url（OSS，有效期 1 天）。',
      ],
    },
    query: {
      id: 'query',
      title: '任务状态查询',
      method: 'GET' as const,
      path: `${v1}/images/tasks_async/{task_id}`,
      geminiPath: `${v1}/tasks_sc/{task_id}`,
      summary: '建议每 30～60 秒轮询一次。OpenAI 用 tasks_async，Gemini 用 tasks_sc。HTTP 200 不代表成功，必须以 status 为准。',
      statuses: [
        { status: 'queued / pending', meaning: '已受理，等待执行' },
        { status: 'processing', meaning: '上游生成 / 上传 OSS / 计费确认中' },
        { status: 'succeeded / completed', meaning: '成功，读取 OSS URL（有效期 1 天）' },
        { status: 'failed', meaning: '失败终态' },
      ],
      queuedExample: `{
  "status": "queued",
  "task_id": "asyncimg_0123456789abcdef"
}`,
      successExample: `{
  "status": "succeeded",
  "task_id": "asyncimg_0123456789abcdef",
  "data": [
    { "url": "https://oss.example.com/images/results/output-1.png" }
  ]
}`,
      failedExample: `{
  "status": "failed",
  "task_id": "asyncimg_0123456789abcdef",
  "fail_reason": "image generation failed"
}`,
    },
    oss: {
      title: 'OSS 链接说明',
      bullets: [
        '仅当 status 为 succeeded 时使用 data[].url。',
        'OSS 链接有效期为 1 天，请在过期前下载或转存。',
        '请勿泄露 API Key；查询必须使用提交任务的同一把 Key。',
      ],
    },
    labels: {
      required: '必填',
      optional: '可选',
      params: '参数说明',
      requestBody: '请求体示例',
      acceptResponse: '受理响应（202）',
      statusTable: '状态说明',
      notes: '补充说明',
      baseUrl: '基础地址',
      copy: '复制',
      copied: '已复制',
    },
  }
}
