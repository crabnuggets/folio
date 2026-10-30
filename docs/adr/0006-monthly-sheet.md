# ADR 0006: Make a monthly sheet the primary accounting model

Status: Accepted, 2026-10-29. Supersedes ADR 0002 and ADR 0003 as requirements for the default user experience; their financial models remain valid for retained legacy data.

The user found the initial application too complicated for a monthly check-in. Use named incoming and outgoing monthly amounts, organized into sections, as the primary model. A credit card amount is the **bill amount assigned to that month**, as explicitly confirmed by the user. The monthly remainder is entered money in minus entered money out, including savings and investment contributions. It does not claim to measure confirmed cash movements, consumption, or a bank balance.

Keep this model separate from legacy postings inside the existing authenticated, transactional owner aggregate. Retain legacy endpoints and lossless export rather than inventing payment dates, account links, or automatic conversions to the new sheet. This preserves history while removing account, statement, payment, plan, claim, and draft administration from the normal UI. Any future migration needs an explicit mapping; overlapping historical card figures cannot be safely inferred.

Monthly recurrence carries item names forward with blank amounts. Optional usual amounts remain suggestions until applied. Historical name suggestions reuse item identities without copying amounts or making a one-off item recurring. These choices replace automatic posting and a general scheduling interface with a small monthly workflow. Go owns totals, duplicate protection, recurrence history, and concurrency; React edits and displays the shared contract.
