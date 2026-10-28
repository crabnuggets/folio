import { ApiClient, type Command, type S } from './api';

/** Retains the committed payment and the link command independently. A failed
 * link can never cause retry to create another cash movement. */
export function settlementWorkflow(api: ApiClient, statement: S['Statement'], amount: string, existing: S['Entry'] | S['EntryInput']) {
  const payment: Command<S['Entry']> | undefined = 'id' in existing ? undefined : api.command<'Entry', S['EntryInput']>('/entries', existing);
  let entryId = 'id' in existing ? existing.id : undefined;
  let link: Command<S['Settlement']> | undefined;
  return {
    get paymentId() { return entryId; },
    async run() {
      if (!entryId && payment) entryId = (await payment.run()).id;
      if (!entryId) throw new Error('Payment reference missing.');
      link ??= api.command<'Settlement', S['SettlementInput']>(`/statements/${statement.id}/settlements`, { entry_id: entryId, amount_minor: amount }, { version: statement.version });
      return link.run();
    },
  };
}
