import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/views/user/ImageWorkbenchView.vue'),
  'utf8',
)

describe('ImageWorkbenchView capability-driven controls', () => {
  it('does not restore client-owned platform option defaults', () => {
    expect(source).not.toContain('const DEFAULTS')
    expect(source).not.toContain("['1024x1024', '1536x1024', '1024x1536']")
    expect(source).not.toContain("['1K', '2K', '4K']")
    expect(source).toContain('size: selectedCapabilityOption(sizeOptions.value, form.size)')
    expect(source).toContain('output_format: selectedCapabilityOption(formatOptions.value, form.format)')
  })

  it('renders only the coarse phases exposed by the public task query', () => {
    expect(source).toContain("{ key: 'queued'")
    expect(source).toContain("{ key: 'processing'")
    expect(source).toContain("{ key: 'succeeded'")
    expect(source).not.toContain("{ key: 'uploading'")
    expect(source).not.toContain("{ key: 'billing'")
  })
})
