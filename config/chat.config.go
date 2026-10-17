package config

import (
	"os"
	"regexp"
	"strings"
)

// ChatConfig: free-form replies (text, voice, reactions, read + typing) and inbound media downloads for Magadh Bot,
// which never holds the Meta token. Callers sign with the existing service key NOTIFY_STATUS_SECRET (the one
// mds-orders already uses for POST /meta-status): no key = the chat routes answer 503.
type ChatConfig struct {
	// HMAC key for X-Notify-Signature (= NOTIFY_STATUS_SECRET)
	Secret string
	// the send token (WHATSAPP_AUTH)
	Token string
	// Graph origin + version (GRAPH_BASE_URL / GRAPH_API_VERSION; only a local stub changes the origin)
	GraphBaseURL string
	GraphVersion string
	// the sending number: WHATSAPP_PHONE_NUMBER_ID, else the id inside WHATSAPP_URL (…/<id>/messages)
	PhoneNumberID string
	// hosts (host or host:port, exact) that POST /chat-message audio_url may fetch from; empty = audio_url refused
	AudioURLHosts []string
}

var pnidInURL = regexp.MustCompile(`/([A-Za-z0-9_-]+)/messages/?$`)

// PhoneNumberIDFromURL: the phone number id in a Graph send URL such as https://graph.facebook.com/v21.0/123/messages
func PhoneNumberIDFromURL(u string) string {
	if m := pnidInURL.FindStringSubmatch(strings.TrimSpace(u)); m != nil {
		return m[1]
	}
	return ""
}

func GetChatConfig() ChatConfig {
	env := GetEnvConfig()
	meta := GetMetaConfig()
	pnid := strings.TrimSpace(os.Getenv("WHATSAPP_PHONE_NUMBER_ID"))
	if pnid == "" {
		pnid = PhoneNumberIDFromURL(env.WHATSAPP_URL)
	}
	hosts := []string{}
	for _, h := range strings.Split(os.Getenv("CHAT_AUDIO_URL_HOSTS"), ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			hosts = append(hosts, h)
		}
	}
	return ChatConfig{
		Secret:        meta.StatusSecret,
		Token:         env.WHATSAPP_AUTH_TOKEN,
		GraphBaseURL:  meta.GraphBaseURL,
		GraphVersion:  meta.GraphVersion,
		PhoneNumberID: pnid,
		AudioURLHosts: hosts,
	}
}
