# Visual design and interaction

Recorded 2026-10-28. Applies to the React web frontend and native Kotlin/Jetpack Compose Android application. This guide documents the design; it does not indicate that either client is implemented.

## Direction and decision status

The user confirmed a bright, clean, minimal interface with Apple-inspired restraint and polish. Revolut is an inspiration for typography, prominent financial figures, rounded cards, navigation, charts, and considered use of colour. Translate these qualities into Folio's own visual language; no particular reference screen or app version has been selected for reproduction.

Confirmed preferences:

- Light surfaces, mostly monochrome, with one ocean-teal accent.
- Selective liquid-glass treatment for navigation and overlays; solid surfaces for financial figures and tables. Glass should be tasteful and sparing.
- A calm overview, generous spacing, and details revealed on demand.
- Monthly personal accounting as the main usage pattern. The user does not expect to open the application most days.
- A consistent Folio appearance across web and Android, retaining Android navigation and gestures.
- Restrained spring transitions and smooth state changes. Motion should make interactions feel polished without distracting from the task.

The font family, colour values, dimensions, and motion timings below are **proposed implementation defaults**, not separately confirmed user choices. Refine them during implementation while preserving the confirmed direction. Dark mode is deferred; the initial design and acceptance checks target the light theme.

## Design for the monthly return

On opening, make the selected reporting month, its review status, data freshness, and outstanding review work immediately understandable. Retain the selected month within an active review session. After a long absence, make the month explicit and offer access to the latest month without silently switching an unfinished review.

Lead with the monthly funding remainder, clearly labelled with its coverage. Keep confirmed inflows/outflows and expected commitments nearby. Recognized spending, allocations, monthly surplus, and observed liquid balance remain distinct, following the [product requirements](01-product.md), [accounting rules](02-accounting-and-data.md), and [funding decision](../adr/0003-funding-boundary-and-reimbursements.md). Prominent amounts must not acquire misleading labels such as "Available balance".

Show a concise review checklist and a contextual next action. Missing statements, unconfirmed drafts, unpaid obligations, and unresolved matches must be easy to find. Keep detailed records, evidence, and calculations one deliberate action away. Essential warnings and incomplete coverage stay visible when details are collapsed.

Use one primary heading and one dominant financial figure or task per screen. Prefer a few useful summary groups to a wall of equally prominent cards. Use spacing and quiet dividers for structure; reserve elevated cards for meaningful grouping. Keep qualifiers close to the figures they explain.

Use concise, neutral language: "Needs review", "Statement missing", "Payment recorded", and "Month closed". A negative figure is not automatically overspending. Closing a month is an explicit review assertion, not proof of exhaustive bank coverage.

Do not introduce daily streaks, engagement prompts, celebratory spending effects, an obligatory merchant feed, or decorative financial tickers. Due dates still deserve clear treatment during an infrequent review. This guide does not add notification or automation scope.

## Colour and surfaces

Use semantic tokens in both implementations. This is the initial palette:

| Token | Value | Role |
| --- | --- | --- |
| `canvas` | `#F6F8F8` | Light neutral page background |
| `surface` | `#FFFFFF` | Cards, forms, financial summaries, tables |
| `surface-muted` | `#EDF2F2` | Quiet grouping and secondary controls |
| `text-primary` | `#142020` | Headings, amounts, body text |
| `text-secondary` | `#526363` | Supporting labels and descriptions |
| `border-subtle` | `#DCE5E5` | Decorative dividers and card edges |
| `border-control` | `#718383` | Input boundaries where needed for recognition |
| `accent` | `#087F8C` | Primary actions, selected navigation, focus, chart emphasis |
| `accent-pressed` | `#066773` | Pressed primary actions |
| `accent-subtle` | `#E5F3F4` | Selected backgrounds with dark teal text |
| `critical` | `#B42318` | Errors/critical conditions, paired with text or icons |
| `warning` | `#8A4B08` | Review warnings, paired with text or icons |

Use white text on solid accent buttons and dark text on pale accent surfaces. Validate actual foreground/background combinations before shipping: opacity, disabled states, and backdrops can change contrast. Subtle decorative borders must not be the sole means of identifying an interactive control.

Ocean teal is the single brand accent. Semantic warning/error colours communicate state rather than adding decoration. Teal must not automatically imply financial health: a positive remainder may still have incomplete coverage. Combine colour with labels, signs, icons, patterns, or line styles.

Keep backgrounds quiet. Avoid large saturated gradients, rainbow category palettes, metallic effects, and glossy financial cards. Charts use neutral series with teal emphasis; series must remain distinguishable without colour alone.

## Typography and icons

Proposed family: **Inter**, bundled locally on web and Android with its licence. Use a system sans-serif fallback while the web font loads. Do not depend on Apple-only fonts to achieve this direction.

Use regular, medium, and semibold weights (400, 500, 600). Prefer weight and spacing over all-caps labels or extra-heavy headings. Enable tabular numerals for financial figures, right-align numeric columns, and keep currency and signs legible. Avoid monospaced body text.

| Role | Web size / line height | Android size / line height |
| --- | --- | --- |
| Primary summary figure | 40 / 48 px | 40 / 48 sp |
| Page heading | 28 / 36 px | 28 / 36 sp |
| Section heading | 20 / 28 px | 20 / 28 sp |
| Body, input, action | 16 / 24 px | 16 / 24 sp |
| Supporting label | 14 / 20 px | 14 / 20 sp |

Implement web sizes as rem-based equivalents at the default root size; respect browser zoom and font preferences. Android text uses sp. Let large amounts wrap or adapt their layout; never clip significant digits or shrink essential labels to fit.

Use one coherent family of simple outline icons with consistent stroke weight and optical size. Pair important actions with text; icon-only controls need accessible names and full hit areas. Avoid decorative financial emoji or mixed icon styles.

## Spacing, shape, and responsive layout

Use a 4-unit spacing scale, commonly 8, 12, 16, 24, 32, and 48. Translate geometry into CSS pixels/rem on web and dp on Android; share visual intent rather than physical pixel counts.

Start with 16-24 units of mobile page padding, 24 units within major cards where space permits, and 24-32 units between sections. Use roughly 16-unit card radii and 10-12 for inputs/buttons. Reserve pills for compact selectors, status chips, and floating navigation. Keep shadows soft and sparse; spacing and subtle borders carry most of the structure.

On wide web layouts, centre a bounded content area (initial maximum 1200 CSS px) with stable navigation. Use two overview columns only when both remain comfortably readable. Narrow screens use a single reading order: primary figure and review needs first. Adapt based on content fit. Floating navigation must not obscure forms, focused controls, or the final content row.

Use solid, right-aligned tables for dense desktop detail. On mobile, use labelled rows/cards or a deliberate scrollable table with accessible headers. Preserve field names, signs, currency, status, and evidence access. Generous spacing must not force every item into an oversized card or hide important information.

## Selective liquid glass

Glass is a treatment for a small set of floating surfaces, not the default card style.

- Appropriate candidates: floating navigation, a compact toolbar over scrolling content, and the outer shell of a transient menu or sheet.
- Keep financial figures, summary cards, table bodies, inputs, statement evidence, and confirmation reading areas solid. An overlay may have a glass frame while its content stays opaque.
- Start with a neutral white tint at about 85-92% opacity, modest backdrop blur (about 12-20 CSS px where supported), a fine edge, and a soft shadow. These are visual starting points, not identical blur parameters for both platforms.
- Avoid stacked glass surfaces and decorative glass layers behind the dashboard. Prefer opaque modal content when translucent navigation is already nearby.
- Avoid liquid distortion, chromatic fringes, animated refraction, and translucent text. Labels, focus indicators, and icons stay crisp over scrolling content.
- Provide an opaque equivalent when blur is unsupported, reduced transparency is requested, contrast requires it, or device performance suffers. Do not assume every platform exposes a reduced-transparency preference; use solid surfaces where needed.

The opaque fallback must preserve hierarchy, geometry, contrast, and interactions. Folio should remain recognizable without blur.

## Motion and feedback

Motion explains where content went, what changed, and whether an action completed. It must not delay financial entry, review, or correction.

| Interaction | Starting treatment |
| --- | --- |
| Press, selection, focus | Immediate response; subtle colour/opacity change around 100-160 ms |
| Expand/collapse or ordinary state change | Short fade and small positional change around 160-240 ms |
| Sheet or navigation transition | Well-damped spring, tuned to settle around 220-320 ms |
| Financial amount update | Display the actual formatted value; optional brief crossfade, without intermediate invented values |
| Loading | Stable placeholder geometry and a short content reveal; avoid repeating decorative shimmer |

These are tuning targets, not delays before an action runs. Web and Compose spring implementations can differ. Keep overshoot negligible, travel short, and transitions interruptible. Avoid bouncing figures, staggered dashboard entrances, parallax, automatic chart drawing, and accounting celebrations. Keyboard focus appears immediately.

Respect web reduced-motion preferences and Android system animation settings. Remove spring travel and nonessential animation in the reduced-motion treatment; use immediate changes or a brief fade where appropriate. Completion must be evident through persistent state or text. Optional Android haptics should be subtle and respect device settings.

## Components and states

Keep component anatomy consistent: label, value, qualifier/status, and a clear detail action where needed. Primary buttons are solid teal; secondary actions are neutral. Use explicit labels such as "Record payment", avoiding language that suggests Folio sends money.

- **Overview:** selected month, review status, funding remainder with coverage, supporting inflows/outflows, and review work. Projections are explicitly labelled and secondary to actuals.
- **Statements:** distinguish new spending, amount due, remaining obligation, and payment status. Evidence and history have a clear detail action.
- **Checklist:** progress includes text and counts, with visible missing-item labels. Completion does not automatically post records or close a month.
- **Forms:** persistent labels, sensible grouping, in-place validation, and clear save/cancel actions. Support localized amount entry and keyboards; placeholders are not labels.
- **Confirmations:** show the affected month, financial impact, and action. Explain close/reopen and correction consequences before submission.
- **Charts:** supplement exact figures and include accessible numeric alternatives. Do not invent category breakdowns or trends unsupported by records. Keep charts static except for useful interaction feedback.

Details need discoverable actions such as "View statements" and "See calculation", not hover-only affordances. Essential warnings remain visible without expansion.

Handle these states in both clients:

- **Loading/cold start:** retain layout, show clear progress text, and explain an extended reconnect in plain language.
- **Empty:** distinguish a new month with no records from missing expected records; offer a relevant next action.
- **Unknown/incomplete:** use "Unknown", "Not recorded", or a qualified subtotal. Never replace an unknown amount with zero.
- **Refreshing/stale:** retain last confirmed data with visible freshness; do not imply it is live or recently reconciled.
- **Saving/retry:** acknowledge submission immediately, prevent duplicate activation, and retain input on failure. Feedback must match the server outcome.
- **Offline/expired session:** explain the recoverable state and offer retry/sign-in. Do not imply unsupported offline writes were saved.
- **Error/conflict:** identify the affected task, preserve safe input, and offer recovery. Explain stale revisions and closed-month restrictions understandably.

## Web and Android implementation

Share semantic tokens, typography roles, icon style, component hierarchy, status wording, and financial meanings. Map these roles to CSS variables and Compose theme values. Separate UI implementations follow [ADR 0001](../adr/0001-platform-and-delivery.md); this guide does not introduce a shared UI runtime or select a component library.

### React web

Use semantic HTML, responsive layouts, visible keyboard focus, logical tab order, and accessible dialogs/menus. Backdrop filtering is progressive enhancement with a solid fallback. Provide hover, focus, pressed, selected, and disabled states. Essential actions work with touch and keyboard as well as a pointer.

Use in-page detail or a side panel where width permits, and an accessible sheet/dialog where it does not. Preserve predictable browser history/back behaviour. Trap focus only in modal surfaces, restore it to the invoking control, and protect unsaved changes appropriately.

### Native Android

Use Jetpack Compose theme/component foundations with Folio colours, typography, and shapes. Use Folio's light theme initially, including when the device uses dark mode; a future dark theme needs separate design and contrast review. Keep system bars legible. Wallpaper-derived dynamic colours should not replace the ocean-teal identity by default.

Retain system back and predictive-back behaviour where supported, gesture navigation, edge-to-edge insets, keyboard handling, native scrolling/selection, accessibility semantics, and the platform file picker. Avoid iOS back chevrons and gestures that conflict with Android. Back dismisses the active overlay before leaving the underlying destination, with appropriate protection for unsaved changes.

Keep top-level destination names and selected states consistent; navigation can adapt between a web rail and Android bottom bar. Respect system gesture areas, keep primary actions within comfortable reach, and keep forms usable with the keyboard open. Blur must not become a minimum-device requirement; use the opaque fallback on unsupported or slower devices.

## Acceptance checks

- Confirm a bright neutral appearance, ocean-teal emphasis, generous hierarchy, restrained elevation, and solid financial reading areas.
- Meet WCAG 2.2 AA contrast targets: 4.5:1 for ordinary text, 3:1 for qualifying large text, and 3:1 for required control boundaries/state indicators. Check glass against actual content underneath.
- Use at least 44 by 44 CSS px hit areas for web touch controls and 48 by 48 dp on Android, with separation between adjacent targets.
- Check keyboard operation, screen-reader labels, TalkBack traversal, visible focus, and text scaling. Verify web reflow at 320 CSS px and large Android font settings without clipped amounts or inaccessible actions. Essential two-dimensional tables may scroll deliberately.
- Verify reduced motion and opaque fallbacks. Interrupted transitions and repeated taps must not obstruct work or imply success prematurely.
- Review the same synthetic reporting month on both clients. Labels, totals, coverage, status, and evidence access must agree despite layout differences.
- Check narrow/wide web layouts and compact Android screens, including a visible keyboard. Floating surfaces must not cover actionable content.
- Exercise loading, empty, incomplete, stale, error, retry, offline, expired-session, correction, and closed-month states.

Use synthetic figures and account aliases in visual examples and screenshots. Document deliberate design changes here so both clients stay aligned.
