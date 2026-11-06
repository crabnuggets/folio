const MIN = -9223372036854775808n;
const MAX = 9223372036854775807n;

/** Exact decimal input → canonical minor units. No floating-point conversions. */
export function parseMoney(input: string, exponent: number, positive = false): string {
  if (!Number.isInteger(exponent) || exponent < 0 || exponent > 4) throw new Error('Unsupported currency precision.');
  const value = input.trim();
  // An explicit decimal point avoids ambiguous thousands/decimal separators.
  const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(value);
  if (!match || (match[3]?.length ?? 0) > exponent) {
    throw new Error(exponent === 2 ? 'Enter a number like 1250.50, without commas.' : `Enter a number with up to ${exponent} decimal places, without commas.`);
  }
  const digits = match[2] + (match[3] ?? '').padEnd(exponent, '0');
  const amount = BigInt((match[1] || '') + digits);
  if (amount < MIN || amount > MAX) throw new Error('Amount exceeds the supported range.');
  if (positive && amount <= 0n) throw new Error('Enter an amount greater than zero.');
  return amount.toString();
}

/** Precision-preserving display; only the whole-unit BigInt reaches Intl. */
export function formatMoney(value: string | null | undefined, currency: string, exponent: number): string {
  if (value == null) return 'Unknown';
  if (!/^(0|-?[1-9][0-9]*)$/.test(value)) return 'Invalid amount';
  const negative = value.startsWith('-');
  const digits = (negative ? value.slice(1) : value).padStart(exponent + 1, '0');
  const whole = exponent ? digits.slice(0, -exponent) : digits;
  const fraction = exponent ? `.${digits.slice(-exponent)}` : '';
  return `${negative ? '−' : ''}${currency} ${new Intl.NumberFormat('en-SG').format(BigInt(whole))}${fraction}`;
}

export function moneyInput(value: string | null | undefined, exponent: number): string {
  if (value == null) return '';
  const negative = value.startsWith('-');
  const digits = (negative ? value.slice(1) : value).padStart(exponent + 1, '0');
  return `${negative ? '-' : ''}${exponent ? `${digits.slice(0, -exponent)}.${digits.slice(-exponent)}` : digits}`;
}

/** Display grouping for a canonical decimal input value; editing stays ungrouped. */
export function groupAmount(value: string): string {
  const match = /^(-?)(\d+)(\.\d+)?$/.exec(value);
  return match ? `${match[1]}${new Intl.NumberFormat('en-SG').format(BigInt(match[2]))}${match[3] ?? ''}` : value;
}

export const monthNames = Array.from({ length: 12 }, (_, i) => new Intl.DateTimeFormat('en', { month: 'long' }).format(new Date(2020, i, 1)));
export const formatMonth = (month: string) => `${monthNames[Number(month.slice(5)) - 1]} ${month.slice(0, 4)}`;
