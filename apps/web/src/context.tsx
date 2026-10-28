import { createContext, useContext } from 'react';
import type { ReactNode } from 'react';
import type { ApiClient, S } from './lib/api';
import type { Values } from './components/Form';
import { parseMoney } from './lib/money';

export type ReviewData = {
  settings: S['Settings']; accounts: S['Account'][]; entries: S['Entry'][]; statements: S['Statement'][];
  claims: S['Claim'][]; drafts: S['Draft'][]; snapshots: S['Snapshot'][]; audit: S['Audit'][];
  plan: S['Plan']; month: S['Month']; summary: S['Summary'];
};
export type Task = { run(): Promise<unknown>; paymentId?: string; body?: string };
export type ReviewContextValue = {
  api: ApiClient; data: ReviewData; month: string; blocked: boolean;
  open(title: string, content: ReactNode): void;
  close(): void;
  perform(label: string, task: Task): Promise<void>;
  money(values: Values, key: string, positive?: boolean): string;
};
export const ReviewContext = createContext<ReviewContextValue | null>(null);
export function useReview() {
  const context = useContext(ReviewContext);
  if (!context) throw new Error('Review context missing');
  return context;
}
export const titleCase = (value: string) => value.replaceAll('_', ' ').replace(/^\w/, c => c.toUpperCase());
export const amountParser = (exponent: number) => (values: Values, key: string, positive = false) => parseMoney(String(values[key] ?? ''), exponent, positive);
export const entryKinds = ['income', 'direct_expense', 'refund', 'reimbursement_receipt', 'transfer', 'envelope_funding', 'envelope_return', 'allocation', 'allocation_withdrawal', 'card_settlement'] as const;
export const planKinds = ['income', 'spending', 'allocation', 'funding_inflow', 'funding_outflow'] as const;
