package metatemplates

import (
	"strings"
	"testing"

	"notify/config"
)

func TestApplyArchived(t *testing.T) {
	rows := []Row{{Name: "a"}, {Name: "b"}, {Name: "b", Language: "hi"}}
	arch := map[string]bool{"b": true}
	out, hidden := applyArchived(rows, arch, false)
	if len(out) != 1 || hidden != 2 || out[0].Archived {
		t.Errorf("hide: %v %d", out, hidden)
	}
	out, hidden = applyArchived(rows, arch, true)
	if len(out) != 3 || hidden != 0 || !out[1].Archived || !out[2].Archived {
		t.Errorf("show: %v %d", out, hidden)
	}
	if rows[1].Archived {
		t.Error("the cached rows were changed")
	}
}

func TestDeleteURL(t *testing.T) {
	cfg := config.MetaConfig{GraphBaseURL: "https://graph.facebook.com", GraphVersion: "v21.0", WabaID: "123"}
	if u := deleteURL(cfg, "sale_close", ""); u != "https://graph.facebook.com/v21.0/123/message_templates?name=sale_close" {
		t.Error(u)
	}
	if u := deleteURL(cfg, "sale_close", "987"); !strings.HasSuffix(u, "?hsm_id=987&name=sale_close") {
		t.Error(u)
	}
}
