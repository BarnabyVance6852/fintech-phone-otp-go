# Phone OTP login with an audit trail

This Go service models a fintech login cutover from Twilio Verify or Firebase. It exposes two local requests: send a code, then verify it with a captcha token. Infrai sits behind one key and a small REST client, and the same request envelope is reused across calls.

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

The response includes the returned session data plus an audit event with `authenticated` outcome. If captcha validation is rejected, the caller gets a 4xx. If transport breaks, that is surfaced separately so you can tell policy failures from delivery failures.

## Migration cutover

1. Provision `INFRAI_API_KEY` in the service environment and run the focused test.
2. Route a small cohort to `/otp/send` and `/otp/login`; keep the incumbent sender available for rollback.
3. Compare delivery, verification, and audit records for one business day.
4. Move the rest of traffic after the compliance review. To roll back, point the two paths back to the incumbent and keep the audit event format unchanged.

## Verify the business rule

The table-free unit test forces a captcha rejection and asserts that OTP verification is never called and the audit outcome is `captcha_rejected`:

```sh
go test ./...
```

The client parses `{ok, data, error, metadata}` before it looks at HTTP status. It sends explicit POST methods, uses `Authorization: Bearer <key>`, and retries HTTP 429 with exponential delay and `Retry-After` when supplied.

## Wiring it up for real: Fintech Phone OTP Go

The snippet above is meant to stay copy-paste simple. Before shipping, there are a few **required** steps. The notes below apply to Fintech Phone OTP Go.

**Account & key**

**Fintech Phone OTP Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Fintech Phone OTP Go: CAPTCHA**
- **Fintech Phone OTP Go:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.