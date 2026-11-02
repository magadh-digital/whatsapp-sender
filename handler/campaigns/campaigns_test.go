package campaigns

import (
	"testing"
	"time"

	"notify/handler/metatemplates"
	"notify/models"
)

func TestPhones(t *testing.T) {
	out, dropped := Phones([]string{"9876543210", "+91 98765 43210", "09812345678", "12345", "919898989898"})
	if len(out) != 3 || out[0] != "9876543210" || out[1] != "9812345678" || out[2] != "9898989898" || dropped != 1 {
		t.Errorf("phones %v dropped %d", out, dropped)
	}
}

func TestApply(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	fresh := func() *models.Campaign {
		return &models.Campaign{Ref: "job-1", Name: "mahan_offer", Language: "en", Status: models.CampaignAwaiting, Phones: []string{"9800000001"}}
	}

	c := fresh()
	if d := Apply(c, &metatemplates.Row{Name: "mahan_offer", Language: "en", Status: "PENDING"}, "", now); d != DecideWait || c.MetaStatus != "PENDING" || c.Status != models.CampaignAwaiting || c.CheckedAt == nil {
		t.Errorf("pending: %s %+v", d, c)
	}

	c = fresh()
	if d := Apply(c, &metatemplates.Row{Name: "mahan_offer", Language: "en", Status: "APPROVED"}, "", now); d != DecideQueue || c.Status != models.CampaignAwaiting {
		t.Errorf("approved must ask to queue and leave the status to queue(): %s %+v", d, c)
	}

	c = fresh()
	if d := Apply(c, &metatemplates.Row{Name: "mahan_offer", Language: "en", Status: "REJECTED", RejectedReason: "TAG_CONTENT_MISMATCH"}, "", now); d != DecideReject || c.Status != models.CampaignRejected || c.RejectedReason != "TAG_CONTENT_MISMATCH" {
		t.Errorf("rejected: %s %+v", d, c)
	}

	c = fresh()
	if d := Apply(c, nil, "No template named mahan_offer on Meta", now); d != DecideReject || c.Status != models.CampaignRejected || c.RejectedReason == "" {
		t.Errorf("gone from Meta: %s %+v", d, c)
	}

	c = fresh()
	if d := Apply(c, nil, "Could not reach Meta: timeout", now); d != DecideRetry || c.Status != models.CampaignAwaiting || c.Error == "" {
		t.Errorf("meta down must keep waiting: %s %+v", d, c)
	}
}

func TestValuesOf(t *testing.T) {
	c := &models.Campaign{Params: []string{"a", "b"}, Header: "h", Buttons: map[string]string{"2": "x", "bad": "y"}}
	v := valuesOf(c)
	if len(v.Body) != 2 || v.Header != "h" || v.Buttons[2] != "x" || len(v.Buttons) != 1 {
		t.Errorf("values %+v", v)
	}
}
