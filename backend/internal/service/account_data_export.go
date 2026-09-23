package service

import (
	"strconv"
	"time"
)

// AccountDataPayload is the portable account export format shared by the
// user-owned export surface and the administrator backup surface. The caller
// is responsible for selecting an ownership scope before building the payload.
type AccountDataPayload struct {
	Type           string               `json:"type,omitempty"`
	Version        int                  `json:"version,omitempty"`
	ExportedAt     string               `json:"exported_at"`
	Proxies        []AccountDataProxy   `json:"proxies"`
	Accounts       []AccountDataAccount `json:"accounts"`
	SkippedShadows int                  `json:"skipped_shadows,omitempty"`
}

type AccountDataProxy struct {
	ProxyKey        string `json:"proxy_key"`
	Name            string `json:"name"`
	Protocol        string `json:"protocol"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	Status          string `json:"status"`
	ExpiresAt       *int64 `json:"expires_at,omitempty"`
	FallbackMode    string `json:"fallback_mode,omitempty"`
	BackupProxyName string `json:"backup_proxy_name,omitempty"`
	ExpiryWarnDays  int    `json:"expiry_warn_days,omitempty"`
}

type AccountDataAccount struct {
	Name               string         `json:"name"`
	Notes              *string        `json:"notes,omitempty"`
	Platform           string         `json:"platform"`
	Type               string         `json:"type"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra,omitempty"`
	ProxyKey           *string        `json:"proxy_key,omitempty"`
	Concurrency        int            `json:"concurrency"`
	Priority           int            `json:"priority"`
	RateMultiplier     *float64       `json:"rate_multiplier,omitempty"`
	ExpiresAt          *int64         `json:"expires_at,omitempty"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired,omitempty"`
	OwnerUserID        *int64         `json:"owner_user_id,omitempty"`
	ShareMode          string         `json:"share_mode,omitempty"`
	ShareStatus        string         `json:"share_status,omitempty"`
	SharePolicyID      *int64         `json:"share_policy_id,omitempty"`
}

// BuildAccountDataPayload only serializes the supplied accounts and proxies.
// It does not query storage or widen visibility, so callers can enforce their
// own admin or user ownership boundary before calling it.
func BuildAccountDataPayload(accounts []Account, proxies []Proxy, proxyKeyBuilder func(protocol, host string, port int, username, password string) string) AccountDataPayload {
	if accounts == nil {
		accounts = []Account{}
	}
	if proxies == nil {
		proxies = []Proxy{}
	}
	if proxyKeyBuilder == nil {
		proxyKeyBuilder = func(protocol, host string, port int, username, password string) string {
			return protocol + "|" + host + "|" + formatPort(port) + "|" + username + "|" + password
		}
	}

	proxyKeyByID := make(map[int64]string, len(proxies))
	proxyNameByID := make(map[int64]string, len(proxies))
	dataProxies := make([]AccountDataProxy, 0, len(proxies))
	for i := range proxies {
		proxy := proxies[i]
		proxyNameByID[proxy.ID] = proxy.Name
		key := proxyKeyBuilder(proxy.Protocol, proxy.Host, proxy.Port, proxy.Username, proxy.Password)
		proxyKeyByID[proxy.ID] = key
		var expiresAt *int64
		if proxy.ExpiresAt != nil {
			value := proxy.ExpiresAt.Unix()
			expiresAt = &value
		}
		backupName := ""
		if proxy.BackupProxyID != nil {
			backupName = proxyNameByID[*proxy.BackupProxyID]
		}
		dataProxies = append(dataProxies, AccountDataProxy{
			ProxyKey:        key,
			Name:            proxy.Name,
			Protocol:        proxy.Protocol,
			Host:            proxy.Host,
			Port:            proxy.Port,
			Username:        proxy.Username,
			Password:        proxy.Password,
			Status:          proxy.Status,
			ExpiresAt:       expiresAt,
			FallbackMode:    proxy.FallbackMode,
			BackupProxyName: backupName,
			ExpiryWarnDays:  proxy.ExpiryWarnDays,
		})
	}

	dataAccounts := make([]AccountDataAccount, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		var proxyKey *string
		if account.ProxyID != nil {
			if key, ok := proxyKeyByID[*account.ProxyID]; ok {
				proxyKey = &key
			}
		}
		var expiresAt *int64
		if account.ExpiresAt != nil {
			value := account.ExpiresAt.Unix()
			expiresAt = &value
		}
		dataAccounts = append(dataAccounts, AccountDataAccount{
			Name:               account.Name,
			Notes:              account.Notes,
			Platform:           account.Platform,
			Type:               account.Type,
			Credentials:        account.Credentials,
			Extra:              account.Extra,
			ProxyKey:           proxyKey,
			Concurrency:        account.Concurrency,
			Priority:           account.Priority,
			RateMultiplier:     account.RateMultiplier,
			ExpiresAt:          expiresAt,
			AutoPauseOnExpired: &account.AutoPauseOnExpired,
			OwnerUserID:        account.OwnerUserID,
			ShareMode:          account.ShareMode,
			ShareStatus:        account.ShareStatus,
			SharePolicyID:      account.SharePolicyID,
		})
	}

	return AccountDataPayload{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Proxies:    dataProxies,
		Accounts:   dataAccounts,
	}
}

func formatPort(port int) string {
	if port < 0 {
		return "0"
	}
	return strconv.Itoa(port)
}
