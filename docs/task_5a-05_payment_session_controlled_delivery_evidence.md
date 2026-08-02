# Payment Session controlled-delivery evidence

## Scope

This record captures the Xendit Test Mode Dashboard `Test and Save` proof for
`payment_session.completed` and `payment_session.expired`. It intentionally
contains no callback-token value, raw body, provider object ID, customer data,
checkout or return URL, payment token, metadata, or provider description.

## Confirmed facts

- Both deliveries reached the hardened Payment Session route and received the
  same generic HTTP `200` response category.
- The callback-token header was present and accepted by the exact constant-time
  verifier. No HMAC, signed timestamp, or rotated-token overlap was observed.
- The observed schema uses top-level `event`, `business_id`, `created`, and
  `data`; it does not use the provisional body `version` field.
- The observed version signal is the `api-version: v1` header.
- Payment Session primary identity is `data.id`; `webhook-id` is not used for
  canonical identity.
- The raw provider body contains additional diagnostic fields, including nested
  objects and arrays. They are bounded at ingress and discarded before
  normalization, audit, metrics, or persistence.

## Initial outcome and correction

The original V1 parser rejected both deliveries as unsupported and the ingress
persisted only sanitized `UNSUPPORTED/INVALID_REQUEST` audit evidence. No inbox
row, payment attempt, booking, capture, refund, or finance state was changed.

The V2 synthetic fixtures reproduce the confirmed shape with only redacted
sentinels. They are the Payment Session parser oracle after this correction;
V1 remains historical provisional evidence for the unrelated payment/refund
surfaces.

## Corrected controlled re-proof

After the V2 parser/fixture correction, the same two Dashboard delivery types
were repeated through the HTTPS tunnel:

- Both returned the generic HTTP `200` category, with no `4xx` or `5xx`
  response observed.
- First deliveries produced two Payment Session inbox facts, each
  `DIAGNOSTIC/RECEIVED` with a distinct exact-body hash.
- A repeated delivery was recorded as a same-body duplicate no-op; it did not
  create a second inbox row or modify the normalized fact.
- A database assertion found no persisted customer, business, token, URL,
  metadata, description, channel-property, or payment-channel diagnostic
  values in the two normalized inbox payloads.
- No `PROCESSING` inbox row was created. The processor remained disabled for
  the entire proof.

This is contract evidence only. It does not promote the provisional contract
to `VERIFIED`; an independent targeted review remains required before Task
5C-05 can resume.
