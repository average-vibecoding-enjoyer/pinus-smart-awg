package smart

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

type ImportLink struct {
	Issuer          string
	claimURL, token string
}
type importPayload struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
	Config  string `json:"config"`
}

func ParseImportLink(raw string) (ImportLink, error) {
	if len(raw) > 2048 {
		return ImportLink{}, errors.New("ссылка слишком длинная")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") || u.RawQuery != "" || u.Path != "/pinus/import" || u.RawPath != "" {
		return ImportLink{}, errors.New("нужна HTTPS-ссылка Pinus вида https://сервер/pinus/import#token=…")
	}
	token := strings.TrimPrefix(u.Fragment, "token=")
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if !strings.HasPrefix(u.Fragment, "token=") || err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != token {
		return ImportLink{}, errors.New("некорректный одноразовый токен")
	}
	issuer := "https://" + u.Host
	return ImportLink{Issuer: issuer, claimURL: issuer + "/api/pinus/import", token: token}, nil
}

// Tokens are sent only in Authorization, never in a URL, log or error text.
func ClaimImportLink(ctx context.Context, link ImportLink) (*conf.Config, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return claimImportLink(ctx, link, client)
}
func claimImportLink(ctx context.Context, link ImportLink, client *http.Client) (*conf.Config, error) {
	if link.claimURL == "" || link.token == "" {
		return nil, errors.New("некорректная ссылка импорта")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link.claimURL, nil)
	if err != nil {
		return nil, errors.New("не удалось создать запрос импорта")
	}
	req.Header.Set("Authorization", "Bearer "+link.token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("не удалось получить профиль по HTTPS; соединение или сертификат не прошли проверку")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("сервер не выдал профиль: ссылка могла истечь или уже использована")
	}
	const limit = 2 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("не удалось прочитать ответ импорта или он слишком большой")
	}
	defer clear(data)
	var payload importPayload
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&payload); err != nil {
		return nil, errors.New("неверный формат ответа импорта")
	}
	if dec.Decode(&struct{}{}) != io.EOF || payload.Version != 1 || len(payload.Config) > 1024*1024 || !conf.TunnelNameIsValid(payload.Name) {
		return nil, errors.New("неподдерживаемый ответ импорта")
	}
	config, err := conf.FromWgQuick(payload.Config, payload.Name)
	if err != nil {
		return nil, errors.New("сервер выдал некорректный AWG-профиль")
	}
	i := config.Interface
	if i.PreUp != "" || i.PostUp != "" || i.PreDown != "" || i.PostDown != "" {
		return nil, errors.New("импорт по ссылке не разрешает команды PreUp/PostUp/PreDown/PostDown")
	}
	return config, nil
}
