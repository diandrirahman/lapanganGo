# Task 4-11 — Exact v1.7 Release Blocker Fix Evidence

## Result

`READY FOR 4-12`

The Task 4-10 P1 blockers were fixed and verified from the clean detached source
commit `3656a65de5f4f4226793e1e545ad409b3562654e`. No production or LIVE database,
credential, or provider integration was used.

## Fixed contracts

- `config.LoadFrom` now applies the Phase-4 monetization prohibition before any
  command can open a database, and the unsafe hard-coded password-reset command
  was removed. A command-source security regression prevents it returning.
- Reconciliation now scopes duplicate/fractional checks to the requested range,
  rejects non-canonical refund owner/venue dimensions, compares OPEX per expense,
  and proves an expense void journal reverses its exact posted journal.
- Migration `019` refuses one-sided `valid_from` or `created_at` seed mutation in
  both raw-SQL and `golang-migrate` paths.
- Platform Finance summary, Expenses, and Journals surfaces retain a visible
  `MODE SIMULASI` warning.
- v1.7 known limitations and rollback documentation now match executable
  behavior; manual evidence hashes are verified by an executable manifest gate.

## Exact-commit verification matrix

| Gate | Actual result |
|---|---|
| Config/command security and startup | PASS |
| Production JWT → active-user → SUPER_ADMIN endpoint matrix | PASS, no required skip |
| Booking concurrency/snapshot rollback on disposable DB | PASS |
| Expense create/retry/post/void exact reversal on disposable DB | PASS |
| Journal/audit atomicity and LIVE guard on disposable DB | PASS |
| Reconciliation boundary suite, including new identity/range/OPEX faults | PASS |
| Migration 019–024 raw-SQL and `golang-migrate` rollback matrix | PASS |
| Expense migration constraints/down-safety | PASS |
| Real login-form browser workflow, desktop/mobile, summary/expense/journal/reversal links | PASS |
| Credential scan | PASS, zero hits |
| Disposable database residue | PASS, baseline `0`, final `0` |

The browser gate uses deterministic intercepted API fixtures and therefore proves
the executable UI workflow, route/auth client behavior, banners, responsive
layout, and journal links. Database-bearing Go gates separately use fresh
disposable PostgreSQL databases and prove real persistence invariants. These two
evidence types are intentionally not represented as a single real-API browser run.

## Evidence artifacts

Sanitized artifacts are retained outside the repository at:

`D:\project\lapangGo_task411_evidence_20260722\`

```text
audit-ledger-disposable.log=6BFF4C1900C1C56D34F49E2B3932EE5AA7089DF82B9BECCB94953FAADF633E49
auth-production-chain-disposable.log=51767A13BF5E945E02DCEF9DA549514D254FE26263177B27216BCD213730329E
booking-disposable.log=08C0F3003FE26368DF929AB3CA7E7B207A232EEB9CF862D45CFC752E9BBAD45C
browser-workflow.json=27FFDD1473406E0F28DE09CBF7832B4C9CD03378588A7E346A7BD56AB73A9C4F
credential-scan.json=39996135BB7B1C31F027BD6B3A46C4E12C6E582B07CFBC42BDE805E11F72F3C8
database-residue.json=75A55BCE265C08781275E500CFD5E3ABF48E3A91E349E6AE373466E2B18E758C
desktop-expenses.png=EBACE4C2D2F09E958B364A1C7AE7B15E0106D1C82FDD767CA373B6077D8222D6
desktop-journals.png=EA28EE9F1F150029C514E5AA6EC627986643F801F4E0E0E6D755C9A3A9BD55F7
desktop-summary.png=4A4AF3BDD50047F53D8A548B83E9ED8A316C8AE86D706FA728E232E53E2D6544
desktop-trace.zip=08F51D6B68176EA5FD53E7D56A8DD1DF0EB787F3095D0D7DFCF941AEA839C6C6
evidence-summary.json=EAE1E17C38AFB2A2C6EBC2B5AD25248F5F61D82DEE3A82F1D1230E32AC470F16
expense-migration-disposable.log=5682F9BCDD38962B76C104818E6464099E0F2095B743B4590E9996D4B40A291C
expense-service-disposable.log=2C6E970D13EA46102B1F4CD34F51907FC1D9B622A83346219253200BC89B6801
frontend-browser.log=D3B38EF880350515A87C9A8BF1FAB4C295C21DE32E76A6EEBEECD79B96B1E069
mobile-expenses.png=B15C1E46FAFECF78C58CBD85309CECD8C6956DAD6C7E81C0E6174F0536AAD510
mobile-journals.png=6F9606E8C202D2A8F0FA31E4AD05D7B2BAE1034AD0D955862D7C6A966E6073B3
mobile-summary.png=54A79D131ED499C57F272752D8658F677B11B0D709C4738208C2B77A23690442
mobile-trace.zip=65FF135E56E6A712D9FC27906DA01EA48A7AB17B458BCBF1FA5854DE95639344
reconciliation-disposable.log=84B316770BD19413DDA0C2C641DDB98AC5DD596D0509A48FB3E972D7BABCE1DF
rollback-disposable.log=BA59138FF2021ED66981F4CC20F80B795107CC99761403AC6B90854E155A51B9
security-auth-config.log=6690F67C5C5224FA204D67F02AC0C26ADECB21D8CA5544A4559B6D9B05ACD492
```

The SHA-256 of `sha256-manifest.json` is
`42A75245A7B95DB6CE2E551A8F8FF2D6AA16B15E5E3781E4C02EC277C2ED154F`.

## Repeatable gates

```powershell
./scripts/test_task4_manual_evidence.ps1
./scripts/verify_task4_manual_evidence.ps1
./scripts/verify_task4_release_docs.ps1
```

All three return PASS. The evidence verifier includes a one-byte mutation
negative regression and fails closed on missing files, empty manifests, or hash
mismatch.
