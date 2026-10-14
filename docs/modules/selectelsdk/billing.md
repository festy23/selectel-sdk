# Billing

The account-scoped Billing client preserves existing Balances/Summary/Consumption
contracts and adds Prediction, Projects, Objects, DetailedReport and
TransactionsPage. All methods are safe reads and return no mutation Receipt.
Eight operation bindings cover the four pinned balance/statistics/report/
transactions schemas. Verification is offline, with no account/live requests.

Prediction values are the provider's remaining-runtime estimates in hours,
never money. Nullable integer estimates distinguish omitted, null and zero.
Projects/Objects repeat provider_keys as published, preserve optional locale
omission and retain parent/project/type metadata without adopting infrastructure
refs. Their unpaginated response is bounded by MaxJSONBytes and CallTimeout;
no snapshot consistency is promised.

Reports and transactions use StaticToken only, with X-Token and Selectel
credential audience. Existing balance/statistics methods keep account IAM and
the explicit StaticToken alternative. An incompatible AuthMode is rejected;
there is no credential, scope or endpoint fallback.

## Endpoint disposition

The current [reports page](https://docs.selectel.ru/api/billing-reports/),
[transactions page](https://docs.selectel.ru/api/billing-transactions/), their
published JSON schemas and official SDK clones were checked on 2026-10-14.
The schemas omit servers/host; both rendered pages and HTML publish the paths
without a host/cURL recipe. No matching report/transaction endpoint binding was
found in the checked official SDK inventory. This bounded search does not prove
that an official endpoint is absent or that the service is unavailable.
Hashes and checked URLs are recorded in
`catalog/contracts/billing-endpoint-evidence.json`.

DetailedReport therefore requires a caller-selected Config.EndpointOverrides
binding with service=billing, generation=v1 and the exact account scope.
TransactionsPage requires the analogous v2 binding. Without that explicit
binding, endpoint_unavailable occurs before credential lookup or I/O. Overrides
also apply to other Billing methods of the same generation; callers must choose
the intended endpoint for those calls. No undocumented default host is guessed.

## Financial and protected data

All integral IDs/quantities/amounts retain arbitrary precision. New Amount fields
carry source-described kopeck units without currency conversion or an inferred
account currency; existing Money fields keep their original unit semantics.
Report project identifiers retain either integer or string form. Recursive
objects/resources and all paid-by-balance levels are typed and retained.

Private project/object names, transaction descriptions (including additional
language keys), service context, debt reasons and counterparty labels are
protected from ordinary formatting/serialization. OptionalText preserves null
and omission while its formatter prevents the byte/hex representation leak of
an unprotected generic wrapper. Explicit Value/Reveal is the caller-directed
boundary. Numeric financial fields remain available as typed values.

ProviderTimestamp keeps raw text. UTC is present only for a provider-supplied
offset; timezone-free official examples never receive an invented timezone.
The Transactions source requires server_meta.ru in its schema while its own
200 example omits that field. The typed decoder supports that explicitly
published English-only example; other required fields remain validated.

## Pagination and limits

TransactionsPage returns one caller-selected page, with required limit 1..500,
optional arbitrary-precision nonnegative offset, balances and without_removed.
Optional zero/false values are retained; no offset/default locale is invented.
The source has no next/total/end marker, so even a short page does not claim
completion. Callers explicitly select subsequent offsets. An over-limit provider
response retains the bounded prefix and returns response_limit.

Reports require the source's year2020..2030/month1..12 bounds and optional ru/en
locale. A returned report account_id must exactly match the selected account.
All response buffering uses MaxJSONBytes; dates, unsupported parameters and
account-scope errors are rejected before resource I/O. Provider messages are not
copied into SDK errors or observers. Global source inventories remain incomplete,
so global coverage_ratio stays null despite this enumerated Billing baseline.
