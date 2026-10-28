import { ApiClient, isUncertain, type Command, type S } from './api';

/** Retains the committed payment and the link command independently. A failed
 * link can never cause retry to create another cash movement. Each step retains
 * uncertainty until its own result is recovered; completed steps cannot keep a
 * later definitively rejected step locked. */
export function settlementWorkflow(api: ApiClient, statement: S['Statement'], amount: string, existing: S['Entry'] | S['EntryInput']) {
  const payment: Command<S['Entry']> | undefined = 'id' in existing ? undefined : api.command<'Entry', S['EntryInput']>('/entries', existing);
  let entryId = 'id' in existing ? existing.id : undefined;
  let link: Command<S['Settlement']> | undefined;
  let paymentUncertain = false;
  let linkUncertain = false;
  return {
    get paymentId() { return entryId; },
    get uncertain() { return paymentUncertain || linkUncertain; },
    async run() {
      if (!entryId && payment) {
        try {
          entryId = (await payment.run()).id;
          paymentUncertain = false;
        } catch (failure) { paymentUncertain ||= isUncertain(failure); throw failure; }
      }
      if (!entryId) throw new Error('Payment reference missing.');
      link ??= api.command<'Settlement', S['SettlementInput']>(`/statements/${statement.id}/settlements`, { entry_id: entryId, amount_minor: amount }, { version: statement.version });
      try {
        const result = await link.run();
        linkUncertain = false;
        return result;
      } catch (failure) { linkUncertain ||= isUncertain(failure); throw failure; }
    },
  };
}
