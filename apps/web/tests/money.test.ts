import { describe, expect, it } from 'vitest';
import { formatMoney, moneyInput, parseMoney } from '../src/lib/money';

describe('money precision at the user/API boundary', () => {
  it.each([
    ['5000', 2, '500000'], ['0.01', 2, '1'], ['00012.30', 2, '1230'], ['-0.01', 2, '-1'],
    ['-0', 2, '0'], ['123', 0, '123'], ['0.0001', 4, '1'],
    ['92233720368547758.07', 2, '9223372036854775807'], ['-92233720368547758.08', 2, '-9223372036854775808'],
    ['90071992547409.93', 2, '9007199254740993'],
  ])('parses %s at exponent %i exactly', (input, exponent, expected) => {
    expect(parseMoney(input, Number(exponent))).toBe(expected);
  });
  it.each(['', '1,234.56', '1,20', '1e4', '+1', '1.001', '.50', '1.', 'NaN', 'Infinity', '92233720368547758.08', '-92233720368547758.09'])('rejects ambiguous or out-of-range input %s', input => {
    expect(() => parseMoney(input, 2)).toThrow();
  });
  it('requires positive entries without treating unknown as zero', () => {
    expect(() => parseMoney('0', 2, true)).toThrow('greater than zero');
    expect(() => parseMoney('-1', 2, true)).toThrow('greater than zero');
    expect(formatMoney(null, 'SGD', 2)).toBe('Unknown');
    expect(formatMoney(undefined, 'SGD', 2)).toBe('Unknown');
    expect(formatMoney('0', 'SGD', 2)).toBe('SGD 0.00');
  });
  it('formats significant digits beyond Number precision', () => {
    expect(formatMoney('9007199254740993', 'SGD', 2)).toBe('SGD 90,071,992,547,409.93');
    expect(formatMoney('-1', 'SGD', 2)).toBe('−SGD 0.01');
    expect(formatMoney('-9223372036854775808', 'SGD', 2)).toBe('−SGD 92,233,720,368,547,758.08');
  });
  it('round-trips API values through edit forms without rounding', () => {
    for (const exponent of [0, 2, 3, 4]) for (const value of ['0', '-1', '9007199254740993', '9223372036854775807', '-9223372036854775808']) {
      expect(parseMoney(moneyInput(value, exponent), exponent)).toBe(value);
    }
    expect(moneyInput(null, 2)).toBe('');
  });
});
