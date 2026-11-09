import { describe, expect, it } from 'vitest'
import { currencySymbol, formatPaymentAmount, paymentCurrencyFractionDigits } from '../currency'

describe('formatPaymentAmount', () => {
  it('formats zero- and two-decimal payment currencies', () => {
    expect(formatPaymentAmount(100, 'JPY', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'KRW', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'HKD', 'en-US')).toContain('.00')
    expect(formatPaymentAmount(20, 'USDT', 'en-US')).toBe('20.00 USDT')
  })

  it.each([
    [' iqd ', 3, '100.123'],
    ['CLF', 2, '100.12'],
    ['UYW', 2, '100.12'],
    ['UGX', 0, '100'],
    ['ISK', 0, '100'],
  ])('uses the gateway precision for %s', (currency, digits, expected) => {
    expect(paymentCurrencyFractionDigits(currency)).toBe(digits)
    const renderedAmount = formatPaymentAmount(100.1234, currency, 'en-US')
    expect(renderedAmount.match(/\d+(?:\.\d+)?/)?.[0]).toBe(expected)
  })
})

describe('currencySymbol', () => {
  it('maps common payment currencies and falls back safely', () => {
    expect(currencySymbol('USD')).toBe('$')
    expect(currencySymbol('cny')).toBe('¥')
    expect(currencySymbol('EUR')).toBe('€')
    expect(currencySymbol('')).toBe('¥')
    expect(currencySymbol('XYZ')).toBe('XYZ')
  })
})
