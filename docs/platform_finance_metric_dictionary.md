# v1.7 Platform Finance Metric Dictionary

This dictionary defines the only release-facing meaning of Platform Finance
metrics. It applies to API, UI, reconciliation reports, QA evidence, and
operator communication.

## Contract-wide rules

- Currency is IDR in whole rupiah. Database values are `BIGINT`, Go values are
  `int64`, and API JSON nominal fields are integer-rupiah strings.
- Timezone is `Asia/Jakarta`. A requested date range is inclusive by calendar
  date but queried as UTC half-open `[start, end + 1 day)`.
- The maximum range is 366 calendar days. A `bucket_date` is always a real
  `YYYY-MM-DD`; `period` or `unknown` is not a valid exception bucket.
- `SIMULATION` values are projections or operational facts, not cash received,
  tax, GAAP/IFRS revenue, or a statutory financial statement.
- Actual/unavailable values remain `null` with the matching availability code;
  unavailable is not equivalent to zero.

## Summary metrics

| Field/term | Definition and formula | v1.7 availability |
|---|---|---|
| `online_gmv_gross` / Online GMV Bruto | Sum of realized online booking income in the selected period. | Available from current ledger/source contract. |
| `refund_principal` / Refund Principal | Sum of online refund principal recognized in the selected period. | Available from refund ledger facts. |
| `online_gmv_net` / Online GMV Neto | `online_gmv_gross - refund_principal`. | Available; must not be confused with captured provider funds. |
| `projected_commission` / Proyeksi Komisi | Immutable snapshot/scenario commission for paid online bookings, minus the same original projection on exact full refund. | Available as simulation projection; not revenue. |
| `projected_owner_net_after_hypothetical_commission` | `online_gmv_net - projected_commission`. | Available as hypothetical projection only. |
| `projected_take_rate_bps` | `projected_commission / online_gmv_net * 10,000`; null when net GMV is <= 0. | Available only when denominator is positive. |
| `realized_online_booking_count` | Count of realized online booking facts. | Available. |
| `refunded_booking_count` | Count of refunded online booking facts. | Available. |
| `legacy_manual_realized_gmv` | Historical/manual owner-cash realized GMV kept separate from post-cutover online source. | Available as a separate historical category; never billable commission. |
| `platform_operating_expense` / Platform OPEX | Posted platform expense entries minus the linked reversal effect. | `AVAILABLE` globally; owner/venue-scoped breakdown is `UNAVAILABLE_UNTIL_SCOPE_ALLOCATION` and must not show global OPEX as scoped. |
| `projected_operating_result_before_transaction_costs` | `projected_commission - platform_operating_expense`. | Simulation projection only; excludes gateway/provider tax/payout/refund/chargeback/subsidy costs. |

## Explicitly unavailable fields

| Field/availability | Meaning |
|---|---|
| `gateway_captured_gmv` | `null` / `UNAVAILABLE_UNTIL_GATEWAY`: no provider capture source exists in v1.7. |
| `actual_commission_revenue` / `actual_platform_revenue` | `null` / `UNAVAILABLE_UNTIL_LIVE`: no LIVE revenue recognition is permitted. |
| `payment_processing_expense` | `null` / `UNAVAILABLE_UNTIL_GATEWAY`: no provider-sourced cost exists. |
| `owner_payable` | `null` / `UNAVAILABLE_UNTIL_PLATFORM_COLLECTED`: platform does not control settlement funds. |
| `transaction_contribution` | `null` / `UNAVAILABLE_UNTIL_LIVE`: provider costs and actual revenue are unavailable. |
| `operating_result` | `null` / `UNAVAILABLE_UNTIL_LIVE`: do not substitute projected result or call it net profit. |
| `owner_payout` / Provider Settlement | Unavailable: no payout or settlement flow exists. |

## Projection and classification rules

| Case | Classification |
|---|---|
| Online post-cutover, 7% | `700` bps projection on final-price basis. |
| Online introductory/trial | `500` or `0` bps according to immutable term/snapshot. |
| Offline/walk-in | Always 0% commission and excluded from online GMV/commission. |
| Historical pre-cutover | `LEGACY_NO_COMMISSION`, 0% billable; a separate historical scenario must remain labelled non-billable. |
| Full refund | Exact reversal of principal and original projected commission; do not resolve a new term. |
| OPEX post/void | Posted amount and linked reversal are evaluated in the effective Jakarta date bucket. |

## Reconciliation check vocabulary

The eight checks are:

`ONLINE_LEDGER_GMV_MATCH`, `PAID_SNAPSHOT_SOURCE_MATCH`,
`OFFLINE_ZERO_COMMISSION`, `REFUND_EXACT_REVERSAL`, `NO_DUPLICATE_EVENTS`,
`SUMMARY_BREAKDOWN_TREND_MATCH`, `OPEX_POSTED_REVERSAL_MATCH`, and
`ACTUAL_METRICS_UNAVAILABLE`.

`PASS` means the contract holds. `FAIL` means a dated exception or integrity
delta exists. `BLOCKED` means a safe comparison could not be made and is never a
clean result. Exception evidence must retain `bucket_date`, expected/actual
counts, and expected/actual rupiah; raw database `Reason` text is not a release
artifact.
