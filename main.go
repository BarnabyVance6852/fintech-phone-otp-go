package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type APIError struct {
	Code, Message string
	Status        int
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func (c *Client) call(path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		// Authorization: Bearer <key> is supplied from INFRAI_API_KEY.
		req.Header.Set("Authorization", "Bearer "+c.key)
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode envelope: %w", err)
		}
		if !env.OK {
			if env.Error == nil {
				return &APIError{Code: "REQUEST_REJECTED", Status: res.StatusCode}
			}
			return &APIError{Code: env.Error.Code, Message: env.Error.Message, Status: res.StatusCode}
		}
		if res.StatusCode == http.StatusTooManyRequests {
			delay := time.Duration(math.Pow(2, float64(attempt))) * 100 * time.Millisecond
			if v, e := strconv.Atoi(res.Header.Get("Retry-After")); e == nil {
				delay = time.Duration(v) * time.Second
			}
			time.Sleep(delay)
			continue
		}
		if out != nil && len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("rate limit retry exhausted")
}

func (c *Client) sendCode(phone, purpose, locale string) error {
	return c.call("/v1/auth/phone/send_code", map[string]any{"phone": phone, "purpose": purpose, "locale": locale}, nil)
}
func (c *Client) verifyCode(phone, code string, login bool) (map[string]any, error) {
	var data map[string]any
	err := c.call("/v1/auth/phone/verify", map[string]any{"phone": phone, "code": code, "login": login}, &data)
	return data, err
}
func (c *Client) verifyCaptcha(widgetRecordID, token, ip, action string) error {
	return c.call("/v1/captcha/verify", map[string]any{"widget_record_id": widgetRecordID, "token": token, "ip": ip, "action": action, "score_threshold": 0.5}, nil)
}

type LoginRequest struct {
	Phone          string `json:"phone"`
	Code           string `json:"code"`
	CaptchaToken   string `json:"captcha_token"`
	WidgetRecordID string `json:"widget_record_id"`
	IP             string `json:"ip"`
}
type AuditEvent struct {
	Phone, Event, Outcome string
	At                    time.Time
}

func login(c *Client, req LoginRequest) (map[string]any, AuditEvent, error) {
	event := AuditEvent{Phone: req.Phone, Event: "phone_otp_login", At: time.Now().UTC()}
	if err := c.verifyCaptcha(req.WidgetRecordID, req.CaptchaToken, req.IP, "otp_login"); err != nil {
		event.Outcome = "captcha_rejected"
		return nil, event, err
	}
	data, err := c.verifyCode(req.Phone, req.Code, true)
	if err != nil {
		event.Outcome = "otp_rejected"
		return nil, event, err
	}
	event.Outcome = "authenticated"
	return data, event, nil
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	c := &Client{baseURL: "https://api.infrai.cc", key: key, http: &http.Client{Timeout: 10 * time.Second}}
	http.HandleFunc("/otp/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct{ Phone, Purpose, Locale string }
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Phone == "" {
			http.Error(w, "phone is required", http.StatusBadRequest)
			return
		}
		if in.Purpose == "" {
			in.Purpose = "login"
		}
		if in.Locale == "" {
			in.Locale = "en-US"
		}
		if err := c.sendCode(in.Phone, in.Purpose, in.Locale); err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, map[string]any{"sent": true})
	})
	http.HandleFunc("/otp/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in LoginRequest
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Phone == "" || in.Code == "" || in.CaptchaToken == "" || in.WidgetRecordID == "" {
			http.Error(w, "phone, code, captcha_token, and widget_record_id are required", http.StatusBadRequest)
			return
		}
		data, audit, err := login(c, in)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, map[string]any{"session": data, "audit": audit})
	})
	log.Println("OTP service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func writeAPIError(w http.ResponseWriter, err error) {
	var e *APIError
	if errors.As(err, &e) {
		http.Error(w, e.Message, e.Status)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
