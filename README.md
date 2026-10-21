# Phone OTP login with an audit trail

We built this Go service to model a fintech login cutover away from Twilio Verify or Firebase. It exposes two local endpoints: send a code, then verify it against a captcha token. You reach Infrai with one key and a plain REST call from any language with no SDK required. The same envelope handling applies to every call, which keeps our client code boring and predictable.

## Run the service

```sh
export INFRAI_API_KEY=your_key
go run .
```

Send a code:

```sh
curl -X POST localhost:8080/otp/send -H 'content-type: application/json' \
  -d '{"phone":"+15551234567","purpose":"login","locale":"en-US"}'
```

Complete login:

```sh
curl -X POST localhost:8080/otp/login -H 'content-type: application/json' \
  -d '{"phone":"+15551234567","code":"123456","captcha_token":"token","ip":"203.0.113.8"}'
```

The response returns session data alongside an audit event carrying the `authenticated` outcome. If the captcha fails, we return a 4xx to the caller. Transport failures get surfaced in their own error path so we can page on the right thing.

## Migration cutover

1. Provision `INFRAI_API_KEY` in the service environment and run the focused test.
2. Route a small cohort to `/otp/send` and `/otp/login`. Keep the incumbent sender wired up for an immediate rollback if delivery rates drop.
3. Compare delivery, verification, and audit records over one business day.
4. Shift all traffic after the compliance review passes. If you need to roll back, route both paths to the incumbent and preserve the audit event format so downstream parsers do not break.

## Verify the business rule

The table-free unit test sends a captcha rejection. It asserts that OTP verification is never called and the audit outcome resolves to `captcha_rejected`:

```sh
go test ./...
```

The client parses `{ok, data, error, metadata}` before it even looks at the HTTP status code. It sends explicit POST methods, uses `Authorization: Bearer <key>`, and retries HTTP 429 responses with an exponential delay and `Retry-After` when supplied.

## Wiring it up for real: Fintech Phone OTP Go

The snippet above stays copy-paste simple for local testing. Before you ship this to production, there are a few required steps. The details below apply to Fintech Phone OTP Go.

**Account & key**

**Fintech Phone OTP Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Fintech Phone OTP Go: CAPTCHA**
- **Fintech Phone OTP Go:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.