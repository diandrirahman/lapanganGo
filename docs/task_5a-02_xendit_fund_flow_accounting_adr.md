# ADR 5A-02 — Xendit Sandbox Fund-flow dan Accounting

- Status: **PROVISIONAL — SANDBOX/SHADOW DESIGN ACCEPTED; LIVE APPROVAL DEFERRED**
- Date: 2026-07-23
- Scope: Phase 5 sandbox/shadow only
- Provider: Xendit Test Mode
- Currency: IDR
- Supersedes: none
- Related evidence: `docs/task_5a-01_provider_capability_evidence.md`
- Runtime guard: `PLATFORM_MONETIZATION_ENABLED=false`

## Context

LapangGo adalah marketplace booking venue olahraga. Venue menyediakan layanan
olahraga kepada customer; LapangGo menyediakan discovery, booking, dan calon
fasilitas collection. Phase 5 hanya membangun payment gateway dalam Xendit Test
Mode dan shadow accounting. Tidak ada uang nyata, production settlement,
owner payout, xenPlatform, split payment, atau production journal.

Provider capability evidence menunjukkan Xendit mempunyai hosted checkout,
Payments API, webhook, refund, settlement, sub-account, split, dan KYC
capabilities. Capability tersebut tidak membuktikan bahwa akun LapangGo sudah
mempunyai hak kontraktual untuk mengumpulkan dana bagi banyak venue. Karena
xenPlatform belum diaktifkan dan kontrak marketplace belum disetujui, ADR ini
memisahkan keputusan sandbox yang dapat dibekukan sekarang dari keputusan LIVE
yang tetap menjadi hard blocker.

## Decision drivers

1. Demo harus terlihat seperti payment nyata tetapi hanya memakai dana virtual.
2. Browser redirect tidak boleh menjadi bukti pembayaran.
3. Owner cashbook legacy tidak boleh menerima gateway capture.
4. Customer membayar tepat nilai booking; provider cost ditanggung LapangGo.
5. Komisi baru menjadi revenue saat layanan `COMPLETED`, bukan saat capture.
6. Semua financial facts harus immutable, idempotent, dan dapat direkonsiliasi.
7. LapangGo tidak membangun escrow, penyimpanan kartu, atau custody sendiri.
8. Fund-flow LIVE harus gagal tertutup sampai provider, Finance, dan Legal
   menyetujui model marketplace yang tepat.

## Decisions

### D1 — Peran para pihak

Untuk model bisnis yang direkomendasikan:

- **Seller of record layanan venue:** owner/venue yang menyediakan lapangan dan
  memenuhi booking.
- **Marketplace/operator:** LapangGo, yang menyediakan discovery, booking,
  customer support, serta calon collection agent.
- **PSP merchant account holder pada Phase 5:** akun Xendit Test Mode LapangGo.
  Posisi ini hanya identitas teknis sandbox dan tidak menciptakan status legal
  merchant of record atau kewenangan custody.
- **PSP:** Xendit, yang mensimulasikan penerimaan, status, refund, fee, dan
  settlement dalam Test Mode.

Untuk LIVE, LapangGo hanya boleh mengumpulkan dana atas nama venue bila kontrak
Xendit dan legal opinion secara eksplisit mengizinkannya. Sampai itu tersedia,
merchant-of-record/collection-agent LIVE berstatus **UNAPPROVED** dan seluruh
production collection harus ditolak.

### D2 — Custody dan balance

- Phase 5 tidak mempunyai custody atau customer funds. Saldo Xendit Test Mode
  adalah virtual dan bukan kas/aset produksi.
- LapangGo tidak boleh menyebut saldo virtual sebagai escrow, trust account,
  bank cash, deposit customer, atau dana owner yang dapat dicairkan.
- Tidak ada sub-account owner, split rule, transfer, withdrawal, atau payout.
- `BANK_CASH`, `PSP_CLEARING`, `OWNER_PAYABLE`, dan akun lainnya hanya boleh
  dipakai dalam template ledger terisolasi untuk pengujian; runtime shadow
  production tidak memposting journal tersebut.

### D3 — Owner liability

- Dalam Phase 5 shadow, tidak ada owner payable aktual. Sistem hanya menyimpan
  payment facts dan menghitung perbandingan virtual dari snapshot booking.
- Dalam model LIVE masa depan, liability kepada owner secara konseptual timbul
  ketika capture sudah diverifikasi server-to-server untuk booking
  `PLATFORM_COLLECTED` yang eligible.
- Nilai liability adalah `booking_fee_snapshots.owner_net_amount_rupiah`.
  Provider processing fee, provider tax, refund fee, dan payout fee tidak
  mengurangi entitlement owner pada model awal.
- Liability yang timbul saat capture masih `PENDING`; baru dapat dibayar setelah
  booking `COMPLETED`, provider settlement matched, safety hold berlalu, dan
  tidak ada refund/dispute aktif.
- Full refund yang sukses sebelum payout membalik liability terkait. Approval
  refund saja tidak mengubah liability final sampai provider mengonfirmasi
  refund `SUCCEEDED`.

### D4 — Komisi dan revenue

- Customer charge sama dengan captured booking amount; customer service fee
  tetap nol.
- Commission menggunakan immutable booking snapshot 0%, 5%, atau 7% dari harga
  final setelah promo.
- Capture membentuk deferred/unearned commission secara konseptual, bukan
  revenue.
- Commission revenue baru diakui pada waktu layanan selesai setelah booking
  `COMPLETED`, memakai exact snapshot amount.
- Pada Phase 5 shadow, actual commission revenue selalu `UNAVAILABLE`; tidak ada
  runtime actual journal.

### D5 — Provider fee dan tax

- LapangGo menanggung processing fee, provider tax yang tidak recoverable,
  refund fee, chargeback cost, dan satu scheduled payout fee pada model awal.
- Customer principal dan owner entitlement tidak boleh dikurangi provider cost.
- Nilai actual hanya boleh berasal dari provider cost/settlement fact; public
  pricing atau kalkulasi lokal hanya estimate dan tidak boleh diposting.
- `PROCESSING_FEE` dipetakan ke `PAYMENT_PROCESSING_EXPENSE`.
- `REFUND_FEE` dipetakan ke `REFUND_FEE_EXPENSE`.
- Chargeback loss dipetakan ke `CHARGEBACK_LOSS` setelah dispute contract
  tersedia.
- `PROVIDER_TAX` tetap memiliki subtype fact terpisah. Mapping production-nya
  harus disetujui Finance berdasarkan contracting entity dan apakah pajak dapat
  dikreditkan. Sampai keputusan itu ditandatangani, provider tax hanya boleh
  diuji sebagai bagian `PAYMENT_PROCESSING_EXPENSE` dalam isolated test ledger.
- Credit side mengikuti provider evidence: gunakan `PSP_CLEARING` bila biaya
  dipotong dari provider balance, atau `ACCOUNTS_PAYABLE` bila ditagih terpisah.
  Jangan hard-code salah satunya tanpa settlement/billing fact.

### D6 — Refund dan chargeback

- Ordinary refund Phase 5 adalah full refund only dan harus sama persis dengan
  captured customer amount.
- Refund request bersifat asynchronous: `REQUESTED/PROCESSING` bukan bukti uang
  kembali; hanya provider-confirmed `SUCCEEDED` yang menjadi refund fact final.
- Sebelum completion, full refund membalik owner entitlement dan unearned
  commission secara exact.
- Setelah completion atau payout, kasus menjadi exceptional refund/dispute dan
  tidak boleh mengedit journal atau payout lama.
- Card chargeback hanya boleh disimulasikan. Card tidak boleh diaktifkan pada
  pilot LIVE sampai kontrak menentukan evidence deadline, liability allocation,
  fee, recourse terhadap owner, dan negative-balance treatment.

### D7 — Settlement dan payout

- Tidak ada settlement atau payout aktual pada Phase 5.
- Test Mode balance movement tidak boleh membuat `provider_settlements`,
  `owner_payouts`, `BANK_CASH`, atau laporan produksi.
- Model konseptual LIVE memisahkan:
  1. provider settlement: PSP balance menuju rekening tujuan yang disetujui;
  2. owner payout: pelunasan `OWNER_PAYABLE` kepada owner.
- Payout bukan platform expense dan bukan income owner kedua.
- Rekening settlement, sub-account strategy, payout destination, cadence, dan
  maker-checker tetap scope Phase 6 serta memerlukan provider/legal approval.

## Fund-flow diagrams

### Phase 5 sandbox/shadow — approved technical model

```text
Customer memilih BCA VA / QRIS / Card
                 |
                 v
LapangGo backend membuat Xendit Test Payment Session
                 |
                 v
Customer diarahkan ke hosted checkout Test Mode
                 |
                 v
Xendit mensimulasikan payment + virtual balance
                 |
                 v
Verified webhook atau server-to-server inquiry
                 |
                 v
Immutable local payment/refund/provider-cost facts
                 |
                 +----> shadow reconciliation/projection
                 |
                 `----> isolated test-ledger template only

No real cash | No owner payable actual | No settlement | No payout
```

### Future LIVE model — conceptual and blocked

```text
Customer
   |
   | booking amount
   v
Xendit approved marketplace collection balance
   |
   +---- provider fee/tax ----> LapangGo expense
   |
   +---- provider settlement -> approved bank/clearing destination
   |
   `---- owner entitlement ---> OWNER_PAYABLE ---> future owner payout
                     \
                      `-------> deferred commission ---> revenue at completion
```

This future diagram is not authorization. If the approved Xendit contract uses
sub-accounts or split routing, this ADR must be amended before implementation
because custody, settlement destination, fee billing, and journal credit
accounts may change.

## Conceptual journal mapping

The following entries are normative templates for an **explicit isolated test
ledger only** during Phase 5. Runtime shadow must reject actual posting.

| Event | Debit | Credit | Amount/source |
|---|---|---|---|
| Verified capture | `PSP_CLEARING` | `OWNER_PAYABLE`; `UNEARNED_COMMISSION` | Captured amount split by exact immutable snapshot; omit zero commission line |
| Provider processing fee confirmed | `PAYMENT_PROCESSING_EXPENSE` | `PSP_CLEARING` or `ACCOUNTS_PAYABLE` | Exact provider cost fact and billing mode |
| Provider tax confirmed | Temporary isolated mapping: `PAYMENT_PROCESSING_EXPENSE` | `PSP_CLEARING` or `ACCOUNTS_PAYABLE` | Separate `PROVIDER_TAX` fact; production tax account requires Finance approval |
| Booking completed | `UNEARNED_COMMISSION` | `COMMISSION_REVENUE` | Exact snapshot commission; effective at service completion |
| Full refund succeeded before completion | `OWNER_PAYABLE`; `UNEARNED_COMMISSION` | `PSP_CLEARING` or `REFUND_CLEARING` | Exact owner share + commission = captured principal |
| Refund fee confirmed | `REFUND_FEE_EXPENSE` | `PSP_CLEARING` or `ACCOUNTS_PAYABLE` | Exact provider fee fact |
| Provider settlement — future only | `BANK_CASH` | `PSP_CLEARING` | Exact matched provider settlement |
| Owner payout — Phase 6 future only | `OWNER_PAYABLE` | Approved cash/clearing account | Exact payable items; fee excluded from owner amount |
| Chargeback after payout — future only | `OWNER_RECEIVABLE`; `COMMISSION_REFUND`; optional `CHARGEBACK_LOSS` | `REFUND_CLEARING`/provider-specific clearing | Contractual liability and exact provider facts required |

All journal templates must balance to Rp0, use integer rupiah, omit zero-value
lines, use stable event keys, and remain append-only. Reversal creates a new
journal; no posted journal is edited or deleted.

## Recognition and reconciliation rules

```text
provider_opening
+ captured
- refunds
- provider_fees_and_tax
- provider_settlements
= provider_closing
```

For a future LIVE environment:

```text
PSP_CLEARING ledger balance
= provider balance not yet settled, adjusted only by identified timing items

OWNER_PAYABLE ledger balance
= outstanding owner payable subledger

captured amount
= owner entitlement + deferred commission
```

Phase 5 shadow compares facts and expected templates but posts nothing. Timing
differences must be labeled pending; unexplained differences may not be forced
to zero. Gateway facts must never call the legacy owner-cash income path, and a
booking may not appear in both marketplace-collected and manual-direct income.

## Alternatives rejected

1. **Activate xenPlatform now.** Rejected because sub-account, split, transfer,
   KYC, payout, and fund-control obligations are not approved.
2. **Collect LIVE funds in the ordinary LapangGo merchant account.** Rejected
   because authority to collect for multiple venues is not contractually
   proven.
3. **Settle directly to each owner in Phase 5.** Rejected because owner KYC,
   payout destination versioning, reconciliation, and maker-checker belong to
   Phase 6.
4. **Treat capture as commission revenue.** Rejected because service is not yet
   earned; commission remains unearned until completion.
5. **Deduct gateway fees from owner entitlement or customer refund.** Rejected
   by the approved business model.
6. **Post sandbox transactions into the production ledger.** Rejected because
   virtual funds are not production assets, liabilities, revenue, or expense.
7. **Use browser return URL as payment authority.** Rejected because redirects
   are user-controlled and non-authoritative.

## Consequences

### Positive

- Demo can exercise realistic payment/refund events without real funds.
- Accounting preserves the distinction between captured funds, owner money,
  deferred commission, earned revenue, cost, settlement, and payout.
- Provider contract changes can be isolated behind the future adapter and
  settlement mapping.
- Owner cashbook remains independent and cannot double count gateway captures.

### Costs and limitations

- Phase 5 cannot demonstrate actual owner settlement or payout.
- Standard public fee evidence is insufficient for production contribution;
  account invoice/settlement facts are required.
- Tax mapping and future clearing credit accounts cannot be production-frozen
  until Finance reviews the contracting entity and provider billing model.
- The ADR must be amended if xenPlatform/sub-account routing becomes the
  approved LIVE fund-flow.

## Required human approvals

The approver must replace `TBD`, add the approval date, and reference a signed
ticket/document. An author or AI agent cannot self-approve these rows.

| Gate | Approver name | Role/authority | Date | Evidence reference | Status |
|---|---|---|---|---|---|
| Finance/accounting treatment | TBD | Authorized Finance/Accounting approver | TBD | TBD | NOT APPROVED |
| Legal merchant/seller-of-record and collection authority | TBD | Authorized Legal approver | TBD | TBD | NOT APPROVED |
| Provider marketplace/fund-flow contract | TBD | Authorized business/provider owner | TBD | TBD | NOT APPROVED |
| Product policy confirmation | TBD | Authorized Product owner | TBD | TBD | NOT APPROVED |

Finance approval must explicitly confirm provider-tax treatment and fee credit
accounts. Legal/provider approval must explicitly confirm whether LapangGo may
collect on behalf of multiple venues and whether xenPlatform/sub-accounts are
required before LIVE.

### Demo approval deferral decision

On 2026-07-23, the project owner directed that named Finance, Legal, Product,
and provider-contract approvals may be deferred while LapangGo remains an
internal Xendit Test Mode demo. This is a scope decision, not an assertion that
the approval rows above have passed.

The deferral permits Phase 5A design work to continue and permits later Test
Mode implementation only after the remaining Phase 5A technical contracts and
the explicit `GO FOR SANDBOX/SHADOW ONLY` gate are complete. It never permits:

- Xendit Live Mode or production credentials;
- real customer funds;
- xenPlatform, sub-account, split, transfer, settlement, or owner payout;
- production actual journal, payable, or revenue;
- a claim that LapangGo is legally authorized to collect for multiple venues.

Before any LIVE planning, credential, payment order, owner activation, or real
fund movement, all approval rows above must be completed with actual name,
authority, date, and evidence reference. Finance must approve accounting/tax;
Legal and the provider-contract owner must approve collection authority and
fund-flow; Product must approve payment/refund policy; Security approval from
Task 5A-04 must also be complete.

## Invariants for the next tasks

- `PLATFORM_MONETIZATION_ENABLED=false` throughout Phase 5.
- Xendit Test Mode only; real money and production credentials prohibited.
- xenPlatform, sub-account, split, transfer, settlement, and payout prohibited.
- Browser callbacks cannot mark paid.
- Runtime shadow cannot create actual payment/commission/refund journals.
- Full refund only; approval is not provider success.
- Provider costs never reduce customer principal or owner entitlement.
- No gateway event writes legacy owner cashbook income.
- Any change to roles, custody, fee allocation, or journal mapping returns to
  this ADR and requires new human approval.

## Gate verdict

The project-owner decision allows the next design task to proceed under the
frozen sandbox/shadow boundary:

**READY TO DRAFT 5A-03 — SANDBOX/SHADOW ONLY**

The production verdict remains:

**PHASE 5 LIVE NO-GO — FINANCE, LEGAL, PRODUCT, SECURITY, AND PROVIDER APPROVALS DEFERRED**

No Test Mode provider implementation or call is authorized by this ADR alone;
that requires completion of the remaining Phase 5A contracts and the explicit
Task 5A-05 human verdict `GO FOR SANDBOX/SHADOW ONLY`.
