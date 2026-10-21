package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginRejectsCaptchaBeforeOTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/captcha/verify" {
			t.Fatalf("OTP API should not run after captcha rejection")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["widget_record_id"] != "widget-id" || payload["token"] != "token" {
			t.Fatalf("missing captcha identifiers: %v", payload)
		}
		if _, ok := payload["vendor"]; ok {
			t.Fatalf("unsupported vendor field: %v", payload)
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"ok":false,"data":null,"error":{"code":"CAPTCHA_SCORE_TOO_LOW","message":"captcha rejected"},"metadata":{}}`))
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, key: "test", http: server.Client()}
	// The decision boundary is deterministic: captcha rejection prevents authentication.
	var request LoginRequest
	if err := json.Unmarshal([]byte(`{"phone":"+15551234567","code":"123456","captcha_token":"token","widget_record_id":"widget-id","ip":"203.0.113.8"}`), &request); err != nil {
		t.Fatal(err)
	}
	_, audit, err := login(c, request)
	if err == nil || audit.Outcome != "captcha_rejected" {
		t.Fatalf("expected captcha rejection, got err=%v audit=%+v", err, audit)
	}
}
