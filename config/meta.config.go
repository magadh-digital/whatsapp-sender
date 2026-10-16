package config

import (
	"os"
	"strings"
)

// MetaConfig holds the optional settings for managing message templates on Meta (list / create).
// None of them is required at boot: when one is missing the template endpoints answer 503 with the
// name of the setting to add, and sending keeps working as before.
type MetaConfig struct {
	// WhatsApp Business Account id (Meta Business Manager > WhatsApp accounts). Templates live on the WABA,
	// not on the phone number that WHATSAPP_URL sends from.
	WabaID string
	// Token with the whatsapp_business_management permission. Defaults to WHATSAPP_AUTH (the send token).
	Token string
	// Graph API version for template calls, e.g. v21.0.
	GraphVersion string
	// Graph API origin; only a local test stub ever changes it. Default https://graph.facebook.com.
	GraphBaseURL string
	// mds-orders session check: POST {app_session_id} -> user, permissions (…/api/v1/auth/validate-session/server).
	AuthValidateURL string
	// Permission that may create templates (super_admin always may). Default order:config:update.
	CreatePermission string
	// Shared secret for POST /meta-status (HMAC-SHA256 of the body, header X-Notify-Signature). Empty = intake off.
	StatusSecret string
}

func GetMetaConfig() MetaConfig {
	GetEnvConfig() // loads .env once

	token := strings.TrimSpace(os.Getenv("WHATSAPP_MANAGE_TOKEN"))
	if token == "" {
		token = GetEnvConfig().WHATSAPP_AUTH_TOKEN
	}

	version := strings.TrimSpace(os.Getenv("GRAPH_API_VERSION"))
	if version == "" {
		version = "v21.0"
	}

	base := strings.TrimRight(strings.TrimSpace(os.Getenv("GRAPH_BASE_URL")), "/")
	if base == "" {
		base = "https://graph.facebook.com"
	}

	perm := strings.TrimSpace(os.Getenv("TEMPLATE_CREATE_PERMISSION"))
	if perm == "" {
		perm = "order:config:update"
	}

	return MetaConfig{
		WabaID:           strings.TrimSpace(os.Getenv("WHATSAPP_WABA_ID")),
		Token:            token,
		GraphVersion:     version,
		GraphBaseURL:     base,
		AuthValidateURL:  strings.TrimSpace(os.Getenv("AUTH_VALIDATE_URL")),
		CreatePermission: perm,
		StatusSecret:     strings.TrimSpace(os.Getenv("NOTIFY_STATUS_SECRET")),
	}
}
